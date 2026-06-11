package app.router;

/** A request handler bound to one path via @Route. */
public interface Handler {
    String handle(String body);
}
