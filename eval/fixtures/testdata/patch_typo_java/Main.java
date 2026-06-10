// Tiny greeting CLI in Java used as a write-mode eval fixture.
//
// Contains a one-character typo inside the greet method that prevents
// compilation. A real LLM driven through codrax --mode=write --write-phase=plan should
// identify it and emit a ChangePlan with Kind="patch" carrying a
// unified diff that fixes only that line. (Full apply requires javac
// at runtime; the plan-stage eval only needs the source file.) The
// comments deliberately avoid spelling the typo literal so the eval
// verdict grep cannot be confused for the defect.

public class Main {
    public static String greet(String name) {
        if (name == null || name.isEmpty()) {
            name = "world";
        }
        retrun "Hello, " + name + "!";
    }

    public static void main(String[] args) {
        if (args.length == 0) {
            System.out.println(greet(""));
            return;
        }
        for (String a : args) {
            System.out.println(greet(a));
        }
    }
}
