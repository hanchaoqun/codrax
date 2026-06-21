param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$CaseFile,

    [Parameter(Position = 1)]
    [int]$Runs = 3,

    [string]$Branch = "",
    [int]$PipelineMaxSteps = 15,
    [switch]$SkipBuild
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
if ($PSVersionTable.PSVersion.Major -lt 7) {
    throw "eval/run.ps1 requires PowerShell 7+; invoke it via `pwsh` or eval\\run.cmd."
}
$PSNativeCommandUseErrorActionPreference = $false
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)
[Console]::InputEncoding = $utf8NoBom
[Console]::OutputEncoding = $utf8NoBom
$OutputEncoding = $utf8NoBom

function Resolve-PathRelativeToRepo {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$RepoRoot
    )
    if ([System.IO.Path]::IsPathRooted($Path)) {
        return (Resolve-Path -LiteralPath $Path).Path
    }
    return (Resolve-Path -LiteralPath (Join-Path $RepoRoot $Path)).Path
}

function Parse-CaseFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $text = Get-Content -LiteralPath $Path -Raw -Encoding UTF8
    $text = $text -replace "`r", ""
    $lines = $text -split "`n"
    $vars = [ordered]@{}

    for ($i = 0; $i -lt $lines.Count; $i++) {
        $line = $lines[$i].TrimEnd()
        if ([string]::IsNullOrWhiteSpace($line)) {
            continue
        }
        if ($line.TrimStart().StartsWith("#")) {
            continue
        }
        if ($line -notmatch '^(?<key>[A-Za-z_][A-Za-z0-9_]*)=(?<rest>.*)$') {
            continue
        }

        $key = $Matches["key"]
        $rest = $Matches["rest"].TrimStart()
        if ([string]::IsNullOrEmpty($rest)) {
            $vars[$key] = ""
            continue
        }

        $quote = $rest[0]
        if ($quote -eq '"' -or $quote -eq "'") {
            $value = $rest.Substring(1)
            while ($true) {
                if ($value.EndsWith([string]$quote)) {
                    $value = $value.Substring(0, $value.Length - 1)
                    break
                }
                $i++
                if ($i -ge $lines.Count) {
                    throw "unterminated quoted value for key '$key' in $Path"
                }
                $value += "`n" + $lines[$i]
            }
            $vars[$key] = $value
            continue
        }

        $vars[$key] = $rest
    }

    return $vars
}

function Split-WhitespaceTokens {
    param([string]$Text)
    if ([string]::IsNullOrWhiteSpace($Text)) {
        return @()
    }
    return @($Text -split '\s+' | Where-Object { $_ -ne "" })
}

function Split-RegexPatterns {
    param([string]$Text)
    if ([string]::IsNullOrWhiteSpace($Text)) {
        return @()
    }
    return @(($Text -replace "`r", "") -split "`n" | Where-Object { $_.Trim() -ne "" })
}

function Convert-DimensionKey {
    param([string]$Text)
    $key = ($Text.ToUpperInvariant() -replace '[^A-Z0-9]+', '_').Trim('_')
    return $key
}

function Get-CurrentBranch {
    param([string]$RepoRoot)
    try {
        $current = (& git -C $RepoRoot branch --show-current 2>$null | Out-String).Trim()
        if (-not [string]::IsNullOrWhiteSpace($current)) {
            return $current
        }
    } catch {
    }
    return "main"
}

function Sanitize-PathComponent {
    param([string]$Text)
    $clean = ($Text -replace "`r", "" -replace "`n", " ").Trim()
    if ([string]::IsNullOrWhiteSpace($clean)) {
        return "case"
    }
    return ($clean -replace '[^\p{L}\p{Nd}._-]', '_')
}

function Normalize-DisplayText {
    param([string]$Text)
    if ($null -eq $Text) {
        return ""
    }
    return (($Text -replace "`r", "" -replace "`n", " ").Trim())
}

function Remove-Ansi {
    param([string]$Text)
    if ($null -eq $Text) {
        return ""
    }
    return [regex]::Replace($Text, "\x1B\[[0-9;]*[A-Za-z]", "")
}

function Extract-AnswerBody {
    param([string]$Text)
    $cleaned = Remove-Ansi $Text
    $match = [regex]::Match($cleaned, "鈹佲攣鈹.")
    if ($match.Success) {
        return $cleaned.Substring($match.Index + $match.Length).Trim()
    }
    return $cleaned.Trim()
}

function Extract-AnswerBodyV2 {
    param([string]$Text)
    $cleaned = Remove-Ansi $Text
    $cleaned = $cleaned -replace "`r", ""
    $separator = "━━━"
    $separatorIndex = $cleaned.LastIndexOf($separator, [System.StringComparison]::Ordinal)
    $hasSeparator = $separatorIndex -ge 0
    $candidate = if ($hasSeparator) {
        $cleaned.Substring($separatorIndex + $separator.Length)
    } else {
        $cleaned
    }

    $lines = New-Object System.Collections.Generic.List[string]
    foreach ($rawLine in ($candidate -split "`n")) {
        $line = [string]$rawLine
        $trimmed = $line.Trim()
        if ([string]::IsNullOrWhiteSpace($trimmed)) {
            $lines.Add("")
            continue
        }
        if ($trimmed -eq "(no result)") {
            continue
        }
        if ($trimmed -match '<think>') {
            continue
        }
        if ($trimmed -match '^\s*💭\s+\[[^\]]+\]') {
            continue
        }
        if ($trimmed -match '^\s*[⠁-⣿].*(正在|已|次工具调用|第\s*\d+\s*轮)') {
            continue
        }
        if ($trimmed -match '^\s*[✓✔✗].*(\d+/\d+|次工具调用|第\s*\d+\s*轮)') {
            continue
        }
        $lines.Add($line)
    }

    $result = (($lines.ToArray()) -join "`n").Trim()
    if (-not $hasSeparator) {
        $looksLikePipelineNoise = $cleaned -match '<think>' -or
            $cleaned -match '\[[a-z_]+-\d+\]' -or
            $cleaned -match '正在准备流水线' -or
            $cleaned -match '\(no result\)'
        if ($looksLikePipelineNoise) {
            return ""
        }
    }
    if ($result -eq "(no result)") {
        return ""
    }
    return $result
}

function Extract-AnswerBodyV3 {
    param([string]$Text)
    $cleaned = Remove-Ansi $Text
    $cleaned = $cleaned -replace "`r", ""
    $separator = ([string][char]0x2501) * 3
    $separatorIndex = $cleaned.LastIndexOf($separator, [System.StringComparison]::Ordinal)
    $hasSeparator = $separatorIndex -ge 0
    $candidate = if ($hasSeparator) {
        $cleaned.Substring($separatorIndex + $separator.Length)
    } else {
        $cleaned
    }

    $lines = New-Object System.Collections.Generic.List[string]
    foreach ($rawLine in ($candidate -split "`n")) {
        $line = [string]$rawLine
        $trimmed = $line.Trim()
        if ([string]::IsNullOrWhiteSpace($trimmed)) {
            $lines.Add("")
            continue
        }
        if ($trimmed -eq "(no result)") {
            continue
        }
        if ($trimmed -match '<think>') {
            continue
        }
        if ($trimmed -match '^\s*(💭\s+)?\[[A-Za-z0-9_-]+-\d+\]') {
            continue
        }
        if ($trimmed -match '^\s*[^\p{L}\p{Nd}``''""\[\]()/_.:-]{1,6}\s*\d+/\d+.*$') {
            continue
        }
        if ($trimmed -match '^\s*[✓✔✗]\s*\d+/\d+.*$') {
            continue
        }
        $lines.Add($line)
    }

    $result = (($lines.ToArray()) -join "`n").Trim()
    if (-not $hasSeparator) {
        $looksLikePipelineNoise = $cleaned -match '<think>' -or
            $cleaned -match '\[[a-z_]+-\d+\]' -or
            $cleaned -match '\d+/\d+' -or
            $cleaned -match '\(no result\)'
        if ($looksLikePipelineNoise) {
            return ""
        }
    }
    if ($result -eq "(no result)") {
        return ""
    }
    return $result
}

function Normalize-AnswerCandidateV4 {
    param([string]$Text)
    $cleaned = (Remove-Ansi $Text) -replace "`r", ""
    $lines = New-Object System.Collections.Generic.List[string]
    foreach ($rawLine in ($cleaned -split "`n")) {
        $line = [string]$rawLine
        $trimmed = $line.Trim()
        if ([string]::IsNullOrWhiteSpace($trimmed)) {
            $lines.Add("")
            continue
        }
        if ($trimmed -eq "(no result)") {
            continue
        }
        if ($trimmed -match '<think>') {
            continue
        }
        if ($trimmed -match '^\s*(馃挱\s+)?\[[A-Za-z0-9_-]+-\d+\]') {
            continue
        }
        if ($trimmed -match '^\s*[^\p{L}\p{Nd}``''""\[\]()/_.:-]{1,6}\s*\d+/\d+.*$') {
            continue
        }
        $lines.Add($line)
    }
    $result = (($lines.ToArray()) -join "`n").Trim()
    if ($result -eq "(no result)") {
        return ""
    }
    return $result
}

function Extract-AnswerBodyV4 {
    param([string]$Text)
    $cleaned = (Remove-Ansi $Text) -replace "`r", ""
    $separatorRegex = [regex]'(?:\u2501){3,}'
    $segments = $separatorRegex.Split($cleaned)
    $candidates = New-Object System.Collections.Generic.List[string]
    foreach ($segment in $segments) {
        $candidate = Normalize-AnswerCandidateV4 $segment
        if (-not [string]::IsNullOrWhiteSpace($candidate)) {
            $candidates.Add($candidate)
        }
    }
    if ($candidates.Count -gt 0) {
        for ($i = $candidates.Count - 1; $i -ge 0; $i--) {
            $candidate = $candidates[$i]
            if ((($candidate -replace '\s+', '')).Length -ge 20) {
                return $candidate
            }
        }
        return $candidates[$candidates.Count - 1]
    }

    $result = Normalize-AnswerCandidateV4 $cleaned
    $looksLikePipelineNoise = $cleaned -match '<think>' -or
        $cleaned -match '\[[a-z_]+-\d+\]' -or
        $cleaned -match '\d+/\d+' -or
        $cleaned -match '\(no result\)'
    if ($looksLikePipelineNoise) {
        return ""
    }
    return $result
}

function Count-RegexMatchesInFile {
    param(
        [string]$Path,
        [string]$Pattern
    )
    if ([string]::IsNullOrWhiteSpace($Path) -or -not (Test-Path -LiteralPath $Path)) {
        return 0
    }
    $content = Get-Content -LiteralPath $Path -Raw -Encoding UTF8
    return ([regex]::Matches($content, $Pattern, [System.Text.RegularExpressions.RegexOptions]::Multiline)).Count
}

function Contains-LiteralInsensitive {
    param(
        [string]$Haystack,
        [string]$Needle
    )
    if ([string]::IsNullOrEmpty($Needle)) {
        return $true
    }
    return $Haystack.IndexOf($Needle, [System.StringComparison]::OrdinalIgnoreCase) -ge 0
}

function Get-Median {
    param([int[]]$Values)
    if ($null -eq $Values -or $Values.Count -eq 0) {
        return 0
    }
    $sorted = @($Values | Sort-Object)
    $index = [int][math]::Floor(($sorted.Count - 1) / 2)
    return $sorted[$index]
}

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $scriptRoot "..")).Path
$casePath = Resolve-PathRelativeToRepo -Path $CaseFile -RepoRoot $repoRoot
$case = Parse-CaseFile -Path $casePath

foreach ($required in @("ID", "QUESTION")) {
    if (-not $case.Contains($required) -or [string]::IsNullOrWhiteSpace([string]$case[$required])) {
        throw "case file must define $required"
    }
}

if ([string]::IsNullOrWhiteSpace($Branch)) {
    $Branch = Get-CurrentBranch -RepoRoot $repoRoot
}

$binaryPath = Join-Path $repoRoot "codrax.exe"
if (-not $SkipBuild) {
    Write-Host "building codrax..."
    Push-Location $repoRoot
    try {
        & go build -o $binaryPath .
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
    } finally {
        Pop-Location
    }
}

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$caseId = Sanitize-PathComponent ([string]$case["ID"])
$outDir = Join-Path $repoRoot ("eval/results/{0}-{1}" -f $caseId, $timestamp)
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

$displayName = Normalize-DisplayText ([string]($case["NAME"]))
$displayQuestion = Normalize-DisplayText ([string]($case["QUESTION"]))
Write-Host ("case: {0}  ({1})" -f $caseId, $displayName)
Write-Host ("question: {0}" -f $displayQuestion)
Write-Host ("runs: {0}" -f $Runs)
Write-Host ("outdir: {0}" -f $outDir)
Write-Host ""

$metricPatterns = [ordered]@{
    tool_read_file            = 'tool=read_file'
    unavailable_tool_attempts = '\[agent\] tool ".*" rejected before execution: not in current tool schema|\[explorer\] tool ".*" rejected: tool ".*" is not available in the current explorer repair state'
    checkpoint_continuation_broad_hint = 'window hint applied .*Checkpoint summary.*DAG-scheduled investigation window'
    closure_only_repeated     = 'phase=midloop_signal .*key="explorer\.mid-loop\.[^"]*closure-only\.[0-9]+"'
    mermaid_source_repair_applied = '\[mermaidcompat\] source repair applied'
    repair_debt_checkpoints   = '\[repair_debt\] checkpoint '
    repair_debt_close_ready_filters = '\[repair_debt\] close_ready filtered_advisory='
    concrete_values           = 'concrete values'
    synthesis_runs            = 'SYNTHESIS prompt'
    function_boundary_push    = 'CRITICAL.*Incomplete'
    enumeration_push          = 'explorer\.mid-loop\.enumeration|Principal Enumeration Rows|系统按已验证证据补充成员|系统按已验证证据补充缺失成员'
    focus_warning             = 'Potential Focus'
    t11_gate_skip             = 'T1.1 gate.*skipping'
    t11_gate_run              = 'T1.1 gate.*running'
    dataflow_intent_lookup    = 'dataflowIntent=lookup'
    dataflow_intent_propagate = 'dataflowIntent=propagate'
    midloop_inject            = 'MIDLOOP inject'
    answer_chain_lines        = 'answer_chain'
}

$results = New-Object System.Collections.Generic.List[object]

for ($run = 1; $run -le $Runs; $run++) {
    $runOut = Join-Path $outDir ("run-{0}.out" -f $run)
    $runMetrics = Join-Path $outDir ("run-{0}.metrics.txt" -f $run)
    $runVerdict = Join-Path $outDir ("run-{0}.verdict" -f $run)
    $runLogDir = Join-Path $outDir ("run-{0}.logs" -f $run)
    New-Item -ItemType Directory -Force -Path $runLogDir | Out-Null

    $cmdArgs = @(
        "--repo", ".",
        "--branch", $Branch,
        "--pipeline-max-steps", [string]$PipelineMaxSteps,
        "--log-level", "debug",
        "--log-dir", $runLogDir,
        "--request", [string]$case["QUESTION"]
    )
    if ($case.Contains("LOG") -and -not [string]::IsNullOrWhiteSpace([string]$case["LOG"])) {
        $cmdArgs += @("--log-text", [string]$case["LOG"])
    }

    Push-Location $repoRoot
    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        $stdoutPath = Join-Path $runLogDir "native.stdout.txt"
        $stderrPath = Join-Path $runLogDir "native.stderr.txt"
        $combinedPath = Join-Path $runLogDir "native.combined.txt"
        $psi = [System.Diagnostics.ProcessStartInfo]::new()
        $psi.FileName = $binaryPath
        $psi.WorkingDirectory = $repoRoot
        $psi.UseShellExecute = $false
        $psi.RedirectStandardOutput = $true
        $psi.RedirectStandardError = $true
        $psi.StandardOutputEncoding = $utf8NoBom
        $psi.StandardErrorEncoding = $utf8NoBom
        foreach ($arg in $cmdArgs) {
            [void]$psi.ArgumentList.Add($arg)
        }
        $proc = [System.Diagnostics.Process]::Start($psi)
        $stdoutTask = $proc.StandardOutput.ReadToEndAsync()
        $stderrTask = $proc.StandardError.ReadToEndAsync()
        $proc.WaitForExit()
        [System.Threading.Tasks.Task]::WaitAll(@($stdoutTask, $stderrTask))
        $stdout = $stdoutTask.Result
        $stderr = $stderrTask.Result
        $exitCode = $proc.ExitCode
        $combinedOutput = if ([string]::IsNullOrEmpty($stderr)) {
            $stdout
        } elseif ([string]::IsNullOrEmpty($stdout)) {
            $stderr
        } else {
            ($stderr.TrimEnd("`r", "`n") + "`n" + $stdout.TrimStart("`r", "`n"))
        }
        Set-Content -LiteralPath $stdoutPath -Value $stdout -Encoding UTF8
        Set-Content -LiteralPath $stderrPath -Value $stderr -Encoding UTF8
        Set-Content -LiteralPath $combinedPath -Value $combinedOutput -Encoding UTF8
    } finally {
        $stopwatch.Stop()
        Pop-Location
    }
    $answerBody = Extract-AnswerBodyV4 $stdout
    Set-Content -LiteralPath $runOut -Value $answerBody -Encoding UTF8

    $logFile = Get-ChildItem -LiteralPath $runLogDir -Filter "codrax-*.log" -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTime -Descending |
        Select-Object -First 1
    $logPath = if ($logFile) { $logFile.FullName } else { "" }

    $metricLines = foreach ($metricName in $metricPatterns.Keys) {
        "{0}={1}" -f $metricName, (Count-RegexMatchesInFile -Path $logPath -Pattern $metricPatterns[$metricName])
    }
    $metricLines = @(
        "exit_code=$exitCode"
        "log_file=$logPath"
    ) + $metricLines
    Set-Content -LiteralPath $runMetrics -Value ($metricLines -join "`n") -Encoding UTF8

    $stripped = ($answerBody -replace '\s+', "")
    $passed = $true
    $reasons = New-Object System.Collections.Generic.List[string]

    if ($stripped.Length -lt 20) {
        $passed = $false
        $reasons.Add(("too_short:{0}chars" -f $stripped.Length))
    }

    foreach ($needle in (Split-WhitespaceTokens ([string]($case["EXPECT_CONTAINS"])))) {
        if (-not (Contains-LiteralInsensitive -Haystack $answerBody -Needle $needle)) {
            $passed = $false
            $reasons.Add(("missing:{0}" -f $needle))
        }
    }

    foreach ($needle in (Split-WhitespaceTokens ([string]($case["EXPECT_NOT_CONTAINS"])))) {
        if (Contains-LiteralInsensitive -Haystack $answerBody -Needle $needle) {
            $passed = $false
            $reasons.Add(("banned:{0}" -f $needle))
        }
    }

    foreach ($regexPattern in (Split-RegexPatterns ([string]($case["EXPECT_MATCHES_REGEX"])))) {
        if (-not [regex]::IsMatch($answerBody, $regexPattern)) {
            $passed = $false
            $reasons.Add(("no_regex_match:{0}" -f $regexPattern))
        }
    }

    foreach ($needle in (Split-WhitespaceTokens ([string]($case["EXPECT_SECTIONS"])))) {
        if (-not (Contains-LiteralInsensitive -Haystack $answerBody -Needle $needle)) {
            $passed = $false
            $reasons.Add(("missing_section:{0}" -f $needle))
        }
    }

    foreach ($dim in (Split-WhitespaceTokens ([string]($case["EXPECT_DIMENSIONS"])))) {
        $dimKey = Convert-DimensionKey $dim
        $valuesKey = "EXPECT_DIMENSION_VALUES_{0}" -f $dimKey
        $regexKey = "EXPECT_DIMENSION_REGEX_{0}" -f $dimKey
        $values = if ($case.Contains($valuesKey)) { [string]($case[$valuesKey]) } else { "" }
        $regexes = if ($case.Contains($regexKey)) { [string]($case[$regexKey]) } else { "" }
        if (-not [string]::IsNullOrWhiteSpace($values)) {
            foreach ($value in (Split-WhitespaceTokens $values)) {
                if (-not (Contains-LiteralInsensitive -Haystack $answerBody -Needle $value)) {
                    $passed = $false
                    $reasons.Add(("missing_dimension:{0}:{1}" -f $dim, $value))
                }
            }
            continue
        }
        if (-not [string]::IsNullOrWhiteSpace($regexes)) {
            foreach ($regexPattern in (Split-RegexPatterns $regexes)) {
                if (-not [regex]::IsMatch($answerBody, $regexPattern)) {
                    $passed = $false
                    $reasons.Add(("missing_dimension_regex:{0}:{1}" -f $dim, $regexPattern))
                }
            }
            continue
        }
        if (-not (Contains-LiteralInsensitive -Haystack $answerBody -Needle $dim)) {
            $passed = $false
            $reasons.Add(("missing_dimension:{0}" -f $dim))
        }
    }

    $verdictText = if ($passed) { "PASS" } else { "FAIL " + ($reasons -join " ") }
    Set-Content -LiteralPath $runVerdict -Value $verdictText -Encoding UTF8
    Write-Host ("run {0}: {1}" -f $run, $verdictText)

    $results.Add([pscustomobject]@{
        run       = $run
        case      = $caseId
        exit_code = $exitCode
        seconds   = [math]::Round($stopwatch.Elapsed.TotalSeconds, 1)
        pass      = $passed
        reasons   = ($reasons -join " ")
        out_dir   = $outDir
        log_file  = $logPath
    })
}

$summaryPath = Join-Path $outDir "summary.md"
$summaryJsonPath = Join-Path $outDir "summary.json"
$passCount = @($results | Where-Object { $_.pass }).Count

$summaryLines = New-Object System.Collections.Generic.List[string]
$summaryLines.Add("# Eval results - $caseId")
$summaryLines.Add("")
$summaryLines.Add("- case: ``$displayName``")
$summaryLines.Add("- question: ``$displayQuestion``")
$summaryLines.Add("- runs: $Runs")
$summaryLines.Add("- timestamp: $timestamp")
$summaryLines.Add("")
$summaryLines.Add("## Verdicts")
$summaryLines.Add("")
$summaryLines.Add("| run | result | reasons |")
$summaryLines.Add("|----:|--------|---------|")
foreach ($result in $results) {
    if ($result.pass) {
        $summaryLines.Add("| $($result.run) | PASS | - |")
    } else {
        $summaryLines.Add("| $($result.run) | FAIL | $($result.reasons) |")
    }
}
$summaryLines.Add("")
$summaryLines.Add("**pass rate: $passCount / $Runs**")
$summaryLines.Add("")
$summaryLines.Add("## Mechanism trace metrics")
$summaryLines.Add("")

$metricHeader = "| metric | " + ((1..$Runs | ForEach-Object { "run $_" }) -join " | ") + " | median |"
$metricDivider = "|--------|" + (((1..$Runs | ForEach-Object { "---" }) -join "|")) + "|------|"
$summaryLines.Add($metricHeader)
$summaryLines.Add($metricDivider)
foreach ($metricName in $metricPatterns.Keys) {
    $values = New-Object System.Collections.Generic.List[int]
    $rowParts = New-Object System.Collections.Generic.List[string]
    $rowParts.Add("| $metricName |")
    foreach ($result in $results) {
        $metricsFile = Join-Path $outDir ("run-{0}.metrics.txt" -f $result.run)
        $metricValue = 0
        if (Test-Path -LiteralPath $metricsFile) {
            $line = Select-String -LiteralPath $metricsFile -Pattern ("^{0}=" -f [regex]::Escape($metricName)) | Select-Object -First 1
            if ($line) {
                $metricValue = [int]($line.Line.Split("=", 2)[1])
            }
        }
        $values.Add($metricValue)
        $rowParts.Add(" $metricValue |")
    }
    $rowParts.Add((" {0} |" -f (Get-Median -Values $values.ToArray())))
    $summaryLines.Add(($rowParts -join ""))
}
$summaryLines.Add("")

Set-Content -LiteralPath $summaryPath -Value ($summaryLines -join "`n") -Encoding UTF8
$results | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $summaryJsonPath -Encoding UTF8

Write-Host ""
Write-Host ("summary written: {0}" -f $summaryPath)
Get-Content -LiteralPath $summaryPath -Encoding UTF8
