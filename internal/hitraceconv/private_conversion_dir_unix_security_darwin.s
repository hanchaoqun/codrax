//go:build darwin && (amd64 || arm64)

#include "textflag.h"

TEXT privateConversionDirDarwinACLInitTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_acl_init(SB)
GLOBL ·privateConversionDirDarwinACLInitTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinACLInitTrampolineAddr(SB)/8, $privateConversionDirDarwinACLInitTrampoline<>(SB)

TEXT privateConversionDirDarwinACLSetFDTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_acl_set_fd_np(SB)
GLOBL ·privateConversionDirDarwinACLSetFDTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinACLSetFDTrampolineAddr(SB)/8, $privateConversionDirDarwinACLSetFDTrampoline<>(SB)

TEXT privateConversionDirDarwinACLGetFDTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_acl_get_fd_np(SB)
GLOBL ·privateConversionDirDarwinACLGetFDTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinACLGetFDTrampolineAddr(SB)/8, $privateConversionDirDarwinACLGetFDTrampoline<>(SB)

TEXT privateConversionDirDarwinACLFreeTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_acl_free(SB)
GLOBL ·privateConversionDirDarwinACLFreeTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinACLFreeTrampolineAddr(SB)/8, $privateConversionDirDarwinACLFreeTrampoline<>(SB)

TEXT privateConversionDirDarwinACLGetFlagsTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_acl_get_flagset_np(SB)
GLOBL ·privateConversionDirDarwinACLGetFlagsTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinACLGetFlagsTrampolineAddr(SB)/8, $privateConversionDirDarwinACLGetFlagsTrampoline<>(SB)

TEXT privateConversionDirDarwinACLAddFlagTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_acl_add_flag_np(SB)
GLOBL ·privateConversionDirDarwinACLAddFlagTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinACLAddFlagTrampolineAddr(SB)/8, $privateConversionDirDarwinACLAddFlagTrampoline<>(SB)

TEXT privateConversionDirDarwinFileSecInitTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_filesec_init(SB)
GLOBL ·privateConversionDirDarwinFileSecInitTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinFileSecInitTrampolineAddr(SB)/8, $privateConversionDirDarwinFileSecInitTrampoline<>(SB)

TEXT privateConversionDirDarwinFileSecSetTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_filesec_set_property(SB)
GLOBL ·privateConversionDirDarwinFileSecSetTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinFileSecSetTrampolineAddr(SB)/8, $privateConversionDirDarwinFileSecSetTrampoline<>(SB)

TEXT privateConversionDirDarwinFileSecFreeTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_filesec_free(SB)
GLOBL ·privateConversionDirDarwinFileSecFreeTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinFileSecFreeTrampolineAddr(SB)/8, $privateConversionDirDarwinFileSecFreeTrampoline<>(SB)

TEXT privateConversionDirDarwinMkdirXTrampoline<>(SB),NOSPLIT,$0-0
	JMP libc_codrax_mkdirx_np(SB)
GLOBL ·privateConversionDirDarwinMkdirXTrampolineAddr(SB), RODATA, $8
DATA ·privateConversionDirDarwinMkdirXTrampolineAddr(SB)/8, $privateConversionDirDarwinMkdirXTrampoline<>(SB)
