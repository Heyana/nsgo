package dev.oneo.nsgo;

import java.nio.charset.StandardCharsets;

/**
 * Encodes EventPaste data through Protocol's paste helpers so Go's paste tests
 * verify the Java encoder and Go's readPaste decoder end to end.
 */
public final class PasteCheck {
    public static void main(String[] args) throws Exception {
        StringBuilder data = new StringBuilder();
        for (int i = 0; i < args.length; ) {
            String kind = args[i++];
            switch (kind) {
                case "html":
                    Protocol.appendPasteHTML(data, args[i++]);
                    break;
                case "uri":
                    Protocol.appendPasteURI(data, args[i++]);
                    break;
                case "file": {
                    String mime = args[i++];
                    byte[] bytes = args[i++].getBytes(StandardCharsets.UTF_8);
                    Protocol.appendPasteFile(data, mime, bytes);
                    break;
                }
                case "null-html":
                    Protocol.appendPasteHTML(data, null);
                    break;
                case "empty-html":
                    Protocol.appendPasteHTML(data, "");
                    break;
                default:
                    throw new IllegalArgumentException("unknown paste check op: " + kind);
            }
        }
        System.out.write(data.toString().getBytes(StandardCharsets.UTF_8));
    }
}
