package dev.oneo.nsgo;

import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;
import java.util.Set;

/**
 * Verifies FontFallback CSS parsing, TrueType/OpenType/TTC name table parsing, and CustomFallbackBuilder chain planning on the JVM.
 */
public final class FontFallbackCheck {
    public static void main(String[] args) throws IOException {
        if (args.length == 2 && "parse".equals(args[0])) {
            for (FontFallback.Entry e : FontFallback.parse(args[1])) {
                System.out.println((e.generic ? "generic:" : "named:") + e.name);
            }
            return;
        }
        checkParse();
        checkNameTable();
        checkPlan();
    }

    private static void checkParse() {
        List<FontFallback.Entry> list = FontFallback.parse(
                " Inter , \"Noto Sans SC\", 'Brand, Inc', system-ui, ui-serif, ui-monospace, cursive, fantasy, \"serif\", 'cursive', ");
        assertEq("count", 10, list.size());
        checkEntry(list.get(0), "Inter", false);
        checkEntry(list.get(1), "Noto Sans SC", false);
        checkEntry(list.get(2), "Brand, Inc", false);
        checkEntry(list.get(3), FontFallback.SANS_SERIF, true);
        checkEntry(list.get(4), FontFallback.SERIF, true);
        checkEntry(list.get(5), FontFallback.MONOSPACE, true);
        checkEntry(list.get(6), FontFallback.CURSIVE, true);
        checkEntry(list.get(7), FontFallback.FANTASY, true);
        checkEntry(list.get(8), "serif", false);
        checkEntry(list.get(9), "cursive", false);

        // Case-insensitive generic keywords & Apple aliases.
        List<FontFallback.Entry> generics = FontFallback.parse(
                "SANS-SERIF, -apple-system, BlinkMacSystemFont, ui-sans-serif, ui-rounded, MONOSPACE");
        assertEq("generics count", 6, generics.size());
        checkEntry(generics.get(0), FontFallback.SANS_SERIF, true);
        checkEntry(generics.get(1), FontFallback.SANS_SERIF, true);
        checkEntry(generics.get(2), FontFallback.SANS_SERIF, true);
        checkEntry(generics.get(3), FontFallback.SANS_SERIF, true);
        checkEntry(generics.get(4), FontFallback.SANS_SERIF, true);
        checkEntry(generics.get(5), FontFallback.MONOSPACE, true);

        // Empty and unclosed quote handling.
        assertEq("empty", 0, FontFallback.parse(" , , ").size());
        List<FontFallback.Entry> unclosed = FontFallback.parse("Inter, \"Trailing Unclosed");
        assertEq("unclosed count", 2, unclosed.size());
        checkEntry(unclosed.get(0), "Inter", false);
        checkEntry(unclosed.get(1), "Trailing Unclosed", false);
    }

    private static void checkNameTable() throws IOException {
        // Synthetic TTF with both nameID 16 (Typographic Family) and nameID 1 (Font Family).
        byte[] ttf = buildSfnt(new Object[][] {
                {3, 1, 0x0409, 1, "Noto Sans Condensed"},
                {3, 1, 0x0409, 16, "Noto Sans"},
        });
        List<String> fromBuf = FontFallback.readFamilyNames(ByteBuffer.wrap(ttf), 0);
        assertEq("ttf buffer names", Arrays.asList("Noto Sans", "Noto Sans Condensed"), fromBuf);

        File tmp = File.createTempFile("nsgo-font-check-", ".ttf");
        tmp.deleteOnExit();
        try (FileOutputStream fos = new FileOutputStream(tmp)) {
            fos.write(ttf);
        }
        List<String> fromFile = FontFallback.readFamilyNames(tmp, 0);
        assertEq("ttf file names", Arrays.asList("Noto Sans", "Noto Sans Condensed"), fromFile);

        // Synthetic Mac Roman font (platformID 1, encodingID 0).
        byte[] macTtf = buildSfnt(new Object[][] {
                {1, 0, 0, 1, "LegacyMac"},
        });
        assertEq("mac names", Collections.singletonList("LegacyMac"),
                FontFallback.readFamilyNames(ByteBuffer.wrap(macTtf), 0));

        // Synthetic TTC collection with two distinct faces.
        byte[] face0 = buildSfnt(new Object[][] {{3, 1, 0x0409, 1, "Noto Sans CJK JP"}});
        byte[] face1 = buildSfnt(new Object[][] {{3, 1, 0x0409, 1, "Noto Sans CJK SC"}});
        byte[] ttc = buildTtc(face0, face1);
        assertEq("ttc face 0", Collections.singletonList("Noto Sans CJK JP"),
                FontFallback.readFamilyNames(ByteBuffer.wrap(ttc), 0));
        assertEq("ttc face 1", Collections.singletonList("Noto Sans CJK SC"),
                FontFallback.readFamilyNames(ByteBuffer.wrap(ttc), 1));
        assertEq("ttc out of bounds", Collections.emptyList(),
                FontFallback.readFamilyNames(ByteBuffer.wrap(ttc), 2));

        File tmpTtc = File.createTempFile("nsgo-font-check-", ".ttc");
        tmpTtc.deleteOnExit();
        try (FileOutputStream fos = new FileOutputStream(tmpTtc)) {
            fos.write(ttc);
        }
        assertEq("ttc file face 1", Collections.singletonList("Noto Sans CJK SC"),
                FontFallback.readFamilyNames(tmpTtc, 1));

        // Truncated/corrupt input returns empty list safely.
        assertEq("corrupt", Collections.emptyList(),
                FontFallback.readFamilyNames(ByteBuffer.wrap(new byte[] {0, 1, 2}), 0));
    }

    private static void checkPlan() {
        Map<String, String> app = new HashMap<>();
        app.put("inter", "app:Inter");
        app.put("brandemoji", "app:BrandEmoji");
        app.put("noto sans sc", "app:NotoSC");

        Map<String, String> sys = new HashMap<>();
        sys.put("roboto", "sys:Roboto");
        sys.put("noto serif", "sys:NotoSerif");
        sys.put("droid sans mono", "sys:DroidSansMono");
        sys.put("dancing script", "sys:DancingScript");

        Set<String> aliases = new HashSet<>(Arrays.asList("sans-serif-condensed", "casual"));

        FontFallback.Lookup<String> lookup = new FontFallback.Lookup<String>() {
            @Override
            public String appFamily(String name) {
                return app.get(name.toLowerCase(Locale.ROOT));
            }

            @Override
            public String systemFamily(String name) {
                return sys.get(name.toLowerCase(Locale.ROOT));
            }

            @Override
            public String genericFamily(String genericName) {
                for (String c : FontFallback.genericCandidates(genericName)) {
                    String found = sys.get(c.toLowerCase(Locale.ROOT));
                    if (found != null) return found;
                }
                return null;
            }

            @Override
            public boolean isSystemAlias(String name) {
                return aliases.contains(name.toLowerCase(Locale.ROOT));
            }
        };

        // 1. Registered app font chain with trailing generic.
        FontFallback.Plan<String> p1 = FontFallback.plan(
                FontFallback.parse("Inter, \"Noto Sans SC\", sans-serif"), 64, lookup);
        assertEq("p1 custom", Arrays.asList("app:Inter", "app:NotoSC"), p1.custom);
        assertEq("p1 system", FontFallback.SANS_SERIF, p1.systemFallback);

        // 2. Unknown families skipped before registered and generic families.
        FontFallback.Plan<String> p2 = FontFallback.plan(
                FontFallback.parse("MissingA, Inter, MissingB, monospace"), 64, lookup);
        assertEq("p2 custom", Collections.singletonList("app:Inter"), p2.custom);
        assertEq("p2 system", FontFallback.MONOSPACE, p2.systemFallback);

        // 3. Registered + installed system font + serif fallback.
        FontFallback.Plan<String> p3 = FontFallback.plan(
                FontFallback.parse("inter, \"Noto Serif\", serif"), 64, lookup);
        assertEq("p3 custom", Arrays.asList("app:Inter", "sys:NotoSerif"), p3.custom);
        assertEq("p3 system", FontFallback.SERIF, p3.systemFallback);

        // 4. Mid-chain generic before a custom font preserves fallback order.
        FontFallback.Plan<String> p4 = FontFallback.plan(
                FontFallback.parse("serif, BrandEmoji, sans-serif"), 64, lookup);
        assertEq("p4 custom", Arrays.asList("sys:NotoSerif", "app:BrandEmoji"), p4.custom);
        assertEq("p4 system", FontFallback.SANS_SERIF, p4.systemFallback);

        // 5. Two generic families: first promoted to custom Family, second becomes system fallback.
        FontFallback.Plan<String> p5 = FontFallback.plan(
                FontFallback.parse("serif, monospace"), 64, lookup);
        assertEq("p5 custom", Collections.singletonList("sys:NotoSerif"), p5.custom);
        assertEq("p5 system", FontFallback.MONOSPACE, p5.systemFallback);

        // 6. Generic followed only by missing families stays the terminal system fallback.
        FontFallback.Plan<String> p6 = FontFallback.plan(
                FontFallback.parse("Inter, monospace, TotallyMissing"), 64, lookup);
        assertEq("p6 custom", Collections.singletonList("app:Inter"), p6.custom);
        assertEq("p6 system", FontFallback.MONOSPACE, p6.systemFallback);

        // 7. System alias preserved as fallback.
        FontFallback.Plan<String> p7 = FontFallback.plan(
                FontFallback.parse("Inter, sans-serif-condensed"), 64, lookup);
        assertEq("p7 custom", Collections.singletonList("app:Inter"), p7.custom);
        assertEq("p7 system", "sans-serif-condensed", p7.systemFallback);

        // 8. Deduplication and maxCustom cap.
        FontFallback.Plan<String> p8 = FontFallback.plan(
                FontFallback.parse("Inter, Inter, \"Noto Sans SC\", \"Noto Serif\", sans-serif"), 2, lookup);
        assertEq("p8 custom", Arrays.asList("app:Inter", "app:NotoSC"), p8.custom);
        assertEq("p8 system", FontFallback.SANS_SERIF, p8.systemFallback);

        // 9. Missing families only -> empty custom + sans-serif fallback.
        FontFallback.Plan<String> p9 = FontFallback.plan(
                FontFallback.parse("NopeOne, NopeTwo"), 64, lookup);
        assertEq("p9 custom", Collections.emptyList(), p9.custom);
        assertEq("p9 system", FontFallback.SANS_SERIF, p9.systemFallback);
    }

    /** Builds a minimal valid SFNT containing a 'name' table with the given records. */
    private static byte[] buildSfnt(Object[][] records) throws IOException {
        ByteArrayOutputStream strings = new ByteArrayOutputStream();
        int[] offsets = new int[records.length];
        int[] lengths = new int[records.length];
        for (int i = 0; i < records.length; i++) {
            int platform = (Integer) records[i][0];
            String text = (String) records[i][4];
            byte[] encoded = platform == 1
                    ? text.getBytes(StandardCharsets.ISO_8859_1)
                    : text.getBytes(StandardCharsets.UTF_16BE);
            offsets[i] = strings.size();
            lengths[i] = encoded.length;
            strings.write(encoded);
        }
        ByteArrayOutputStream nameTable = new ByteArrayOutputStream();
        DataOutputStream nt = new DataOutputStream(nameTable);
        nt.writeShort(0); // format
        nt.writeShort(records.length);
        nt.writeShort(6 + records.length * 12); // stringOffset
        for (int i = 0; i < records.length; i++) {
            nt.writeShort((Integer) records[i][0]);
            nt.writeShort((Integer) records[i][1]);
            nt.writeShort((Integer) records[i][2]);
            nt.writeShort((Integer) records[i][3]);
            nt.writeShort(lengths[i]);
            nt.writeShort(offsets[i]);
        }
        nt.write(strings.toByteArray());
        byte[] ntBytes = nameTable.toByteArray();

        ByteArrayOutputStream sfnt = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(sfnt);
        out.writeInt(0x00010000); // TrueType
        out.writeShort(1); // numTables
        out.writeShort(16);
        out.writeShort(0);
        out.writeShort(0);
        // Table record for 'name'
        out.writeInt(0x6E616D65);
        out.writeInt(0); // checksum
        out.writeInt(12 + 16); // offset
        out.writeInt(ntBytes.length);
        out.write(ntBytes);
        return sfnt.toByteArray();
    }

    /** Builds a minimal TTC containing the given SFNT faces with adjusted table offsets. */
    private static byte[] buildTtc(byte[]... faces) throws IOException {
        int headerSize = 12 + faces.length * 4;
        int[] faceOffsets = new int[faces.length];
        int cursor = headerSize;
        for (int i = 0; i < faces.length; i++) {
            faceOffsets[i] = cursor;
            cursor += faces[i].length;
        }
        ByteArrayOutputStream outBytes = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(outBytes);
        out.writeInt(0x74746366); // "ttcf"
        out.writeInt(0x00010000); // version 1.0
        out.writeInt(faces.length);
        for (int off : faceOffsets) out.writeInt(off);
        for (int i = 0; i < faces.length; i++) {
            byte[] copy = faces[i].clone();
            // Adjust the single 'name' table offset (at offset 12 + 8 inside each SFNT) to be file-relative.
            int relOff = ((copy[20] & 0xff) << 24) | ((copy[21] & 0xff) << 16)
                    | ((copy[22] & 0xff) << 8) | (copy[23] & 0xff);
            int absOff = faceOffsets[i] + relOff;
            copy[20] = (byte) (absOff >>> 24);
            copy[21] = (byte) (absOff >>> 16);
            copy[22] = (byte) (absOff >>> 8);
            copy[23] = (byte) absOff;
            out.write(copy);
        }
        return outBytes.toByteArray();
    }

    private static void checkEntry(FontFallback.Entry e, String wantName, boolean wantGeneric) {
        assertEq("entry name", wantName, e.name);
        assertEq("entry generic", wantGeneric, e.generic);
    }

    private static void assertEq(String label, Object want, Object got) {
        if (!Objects.equals(want, got)) {
            throw new AssertionError(label + ": got " + got + ", want " + want);
        }
    }
}
