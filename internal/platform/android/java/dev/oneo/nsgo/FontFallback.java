package dev.oneo.nsgo;

import java.io.File;
import java.io.IOException;
import java.io.RandomAccessFile;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * Parses CSS fontFamily lists, reads TrueType/OpenType family names for Android's SystemFonts, and plans Typeface.CustomFallbackBuilder chains. Pure Java so JVM tests can verify parsing, name-table reading and fallback ordering without an Android runtime.
 */
final class FontFallback {
    static final String SANS_SERIF = "sans-serif";
    static final String SERIF = "serif";
    static final String MONOSPACE = "monospace";
    static final String CURSIVE = "cursive";
    static final String FANTASY = "fantasy";

    /** One parsed family from a CSS fontFamily list. */
    static final class Entry {
        final String name;
        final boolean generic;

        Entry(String name, boolean generic) {
            this.name = name;
            this.generic = generic;
        }
    }

    /** Resolves families during fallback planning. */
    interface Lookup<F> {
        F appFamily(String name);
        F systemFamily(String name);
        F genericFamily(String genericName);
        boolean isSystemAlias(String name);
    }

    /** A planned CustomFallbackBuilder chain: ordered custom families plus the trailing system fallback. */
    static final class Plan<F> {
        final List<F> custom;
        final String systemFallback;

        Plan(List<F> custom, String systemFallback) {
            this.custom = custom;
            this.systemFallback = systemFallback;
        }
    }

    private static final class Kept<F> {
        final F custom;
        final String generic;

        Kept(F custom, String generic) {
            this.custom = custom;
            this.generic = generic;
        }
    }

    private FontFallback() {}

    /**
     * Parses a CSS fontFamily list in preference order, matching Go's driver.FontFamilies plus CSS's cursive and fantasy generic keywords. Quoted names preserve commas and are always treated as named families.
     */
    static List<Entry> parse(String value) {
        List<Entry> out = new ArrayList<>();
        if (value == null) return out;
        int len = value.length();
        int i = 0;
        while (i < len) {
            while (i < len && Character.isWhitespace(value.charAt(i))) i++;
            if (i >= len) break;
            char first = value.charAt(i);
            String name;
            boolean quoted = first == '"' || first == '\'';
            if (quoted) {
                int start = i + 1;
                int end = value.indexOf(first, start);
                if (end < 0) {
                    name = value.substring(start).trim();
                    i = len;
                } else {
                    name = value.substring(start, end).trim();
                    int comma = value.indexOf(',', end + 1);
                    i = comma < 0 ? len : comma + 1;
                }
            } else {
                int comma = value.indexOf(',', i);
                if (comma < 0) {
                    name = value.substring(i).trim();
                    i = len;
                } else {
                    name = value.substring(i, comma).trim();
                    i = comma + 1;
                }
            }
            if (name.isEmpty()) continue;
            String generic = quoted ? null : genericName(name);
            if (generic != null) {
                out.add(new Entry(generic, true));
            } else {
                out.add(new Entry(name, false));
            }
        }
        return out;
    }

    /** Returns the Android generic family name for an unquoted CSS generic keyword, or null. */
    static String genericName(String name) {
        switch (name.toLowerCase(Locale.ROOT)) {
            case "system-ui":
            case "sans-serif":
            case "ui-sans-serif":
            case "-apple-system":
            case "blinkmacsystemfont":
            case "ui-rounded":
                return SANS_SERIF;
            case "serif":
            case "ui-serif":
                return SERIF;
            case "monospace":
            case "ui-monospace":
                return MONOSPACE;
            case "cursive":
                return CURSIVE;
            case "fantasy":
                return FANTASY;
            default:
                return null;
        }
    }

    /** Candidate Android system family names backing each CSS generic family for mid-chain fallbacks. */
    static String[] genericCandidates(String generic) {
        if (generic == null) return new String[0];
        switch (generic) {
            case SANS_SERIF:
                return new String[] {"Roboto", "Noto Sans", "Droid Sans"};
            case SERIF:
                return new String[] {"Noto Serif", "Droid Serif", "Roboto Serif"};
            case MONOSPACE:
                return new String[] {"Droid Sans Mono", "Roboto Mono", "Cutive Mono", "Noto Sans Mono"};
            case CURSIVE:
                return new String[] {"Dancing Script", "Carrois Gothic SC"};
            case FANTASY:
                return new String[] {"Noto Serif", "Roboto"};
            default:
                return new String[0];
        }
    }

    /**
     * Plans a fallback chain for Typeface.CustomFallbackBuilder: resolves app and system families in preference order, skips unknown families, inserts mid-chain generic families when resolved families follow them, caps at maxCustom, and picks the terminal system fallback.
     */
    static <F> Plan<F> plan(List<Entry> entries, int maxCustom, Lookup<F> lookup) {
        List<Kept<F>> kept = new ArrayList<>();
        for (Entry e : entries) {
            if (e.generic) {
                kept.add(new Kept<>(null, e.name));
                continue;
            }
            F f = lookup.appFamily(e.name);
            if (f == null) f = lookup.systemFamily(e.name);
            if (f != null) {
                kept.add(new Kept<>(f, null));
            } else if (lookup.isSystemAlias(e.name)) {
                kept.add(new Kept<>(null, e.name));
            }
        }
        List<F> custom = new ArrayList<>();
        String systemFallback = null;
        int limit = Math.max(0, maxCustom);
        for (int i = 0; i < kept.size(); i++) {
            Kept<F> item = kept.get(i);
            if (item.custom != null) {
                addUnique(custom, item.custom, limit);
                continue;
            }
            if (i == kept.size() - 1) {
                if (systemFallback == null) systemFallback = item.generic;
            } else {
                F g = lookup.genericFamily(item.generic);
                if (g != null) {
                    addUnique(custom, g, limit);
                } else if (systemFallback == null) {
                    systemFallback = item.generic;
                }
            }
        }
        if (systemFallback == null) systemFallback = SANS_SERIF;
        return new Plan<>(custom, systemFallback);
    }

    private static <F> void addUnique(List<F> list, F item, int limit) {
        if (list.size() >= limit) return;
        for (F existing : list) {
            if (existing == item || existing.equals(item)) return;
        }
        list.add(item);
    }

    /**
     * Reads the OpenType/TrueType family names (nameID 16 typographic family and nameID 1 font family) from a memory-mapped or in-memory font buffer. Handles both single SFNT files and TTC font collections via ttcIndex.
     */
    static List<String> readFamilyNames(ByteBuffer data, int ttcIndex) {
        List<String> out = new ArrayList<>(2);
        if (data == null || data.limit() < 12) return out;
        int sfntOffset = 0;
        int tag = getInt(data, 0);
        if (tag == 0x74746366) { // "ttcf"
            if (ttcIndex < 0 || data.limit() < 12) return out;
            int numFonts = getInt(data, 8);
            if (ttcIndex >= numFonts || 12L + (long) ttcIndex * 4L + 4L > data.limit()) return out;
            sfntOffset = getInt(data, 12 + ttcIndex * 4);
        }
        if (sfntOffset < 0 || sfntOffset + 12L > data.limit()) return out;
        int numTables = getU16(data, sfntOffset + 4);
        long dirEnd = (long) sfntOffset + 12L + (long) numTables * 16L;
        if (dirEnd > data.limit()) return out;
        int nameOffset = -1, nameLength = 0;
        for (int i = 0; i < numTables; i++) {
            int rec = sfntOffset + 12 + i * 16;
            if (getInt(data, rec) == 0x6E616D65) { // "name"
                nameOffset = getInt(data, rec + 8);
                nameLength = getInt(data, rec + 12);
                break;
            }
        }
        if (nameOffset < 0 || nameLength < 6 || (long) nameOffset + nameLength > data.limit()) return out;
        byte[] table = new byte[nameLength];
        ByteBuffer slice = data.duplicate();
        slice.position(nameOffset);
        slice.get(table);
        parseNameTable(table, out);
        return out;
    }

    /**
     * Reads the OpenType/TrueType family names from a font file on disk, reading only the header, table directory and name table.
     */
    static List<String> readFamilyNames(File file, int ttcIndex) {
        List<String> out = new ArrayList<>(2);
        if (file == null || !file.isFile()) return out;
        try (RandomAccessFile raf = new RandomAccessFile(file, "r")) {
            long fileLen = raf.length();
            if (fileLen < 12) return out;
            long sfntOffset = 0;
            int tag = raf.readInt();
            if (tag == 0x74746366) { // "ttcf"
                if (ttcIndex < 0) return out;
                raf.skipBytes(4); // version
                int numFonts = raf.readInt();
                if (ttcIndex >= numFonts || 12L + (long) ttcIndex * 4L + 4L > fileLen) return out;
                raf.seek(12L + (long) ttcIndex * 4L);
                sfntOffset = raf.readInt() & 0xffffffffL;
            }
            if (sfntOffset + 12L > fileLen) return out;
            raf.seek(sfntOffset + 4L);
            int numTables = raf.readUnsignedShort();
            if (sfntOffset + 12L + (long) numTables * 16L > fileLen) return out;
            byte[] dir = new byte[numTables * 16];
            raf.seek(sfntOffset + 12L);
            raf.readFully(dir);
            long nameOffset = -1;
            int nameLength = 0;
            for (int i = 0; i < numTables; i++) {
                int rec = i * 16;
                if (readInt(dir, rec) == 0x6E616D65) { // "name"
                    nameOffset = readInt(dir, rec + 8) & 0xffffffffL;
                    nameLength = readInt(dir, rec + 12);
                    break;
                }
            }
            // Cap name table reads at 1 MiB to protect against corrupt files.
            if (nameOffset < 0 || nameLength < 6 || nameLength > (1 << 20) || nameOffset + nameLength > fileLen) {
                return out;
            }
            byte[] table = new byte[nameLength];
            raf.seek(nameOffset);
            raf.readFully(table);
            parseNameTable(table, out);
        } catch (IOException ignored) {
            // Unreadable or malformed font file: leave family names empty.
        }
        return out;
    }

    private static void parseNameTable(byte[] table, List<String> out) {
        if (table.length < 6) return;
        int count = readU16(table, 2);
        int stringOffset = readU16(table, 4);
        if (6 + count * 12 > table.length || stringOffset > table.length) return;
        String preferred = null, preferredFallback = null;
        String family = null, familyFallback = null;
        for (int i = 0; i < count; i++) {
            int rec = 6 + i * 12;
            int platform = readU16(table, rec);
            int encoding = readU16(table, rec + 2);
            int language = readU16(table, rec + 4);
            int nameID = readU16(table, rec + 6);
            int length = readU16(table, rec + 8);
            int offset = readU16(table, rec + 10);
            if (nameID != 1 && nameID != 16) continue;
            if (length <= 0 || stringOffset + offset + length > table.length) continue;
            String decoded = decodeName(table, stringOffset + offset, length, platform, encoding);
            if (decoded == null || decoded.isEmpty()) continue;
            boolean english = (platform == 3 && language == 0x0409)
                    || (platform == 1 && language == 0)
                    || platform == 0;
            if (nameID == 16) {
                if (english && preferred == null) preferred = decoded;
                else if (preferredFallback == null) preferredFallback = decoded;
            } else {
                if (english && family == null) family = decoded;
                else if (familyFallback == null) familyFallback = decoded;
            }
        }
        String n16 = preferred != null ? preferred : preferredFallback;
        String n1 = family != null ? family : familyFallback;
        if (n16 != null) out.add(n16);
        if (n1 != null && (n16 == null || !n1.equalsIgnoreCase(n16))) out.add(n1);
    }

    private static String decodeName(byte[] buf, int off, int len, int platform, int encoding) {
        if ((platform == 3 && (encoding == 1 || encoding == 10)) || platform == 0) {
            return new String(buf, off, len, StandardCharsets.UTF_16BE).trim();
        }
        if (platform == 1 && encoding == 0) {
            return new String(buf, off, len, StandardCharsets.ISO_8859_1).trim();
        }
        return null;
    }

    private static int getInt(ByteBuffer b, int pos) {
        return ((b.get(pos) & 0xff) << 24) | ((b.get(pos + 1) & 0xff) << 16)
                | ((b.get(pos + 2) & 0xff) << 8) | (b.get(pos + 3) & 0xff);
    }

    private static int getU16(ByteBuffer b, int pos) {
        return ((b.get(pos) & 0xff) << 8) | (b.get(pos + 1) & 0xff);
    }

    private static int readInt(byte[] b, int pos) {
        return ((b[pos] & 0xff) << 24) | ((b[pos + 1] & 0xff) << 16)
                | ((b[pos + 2] & 0xff) << 8) | (b[pos + 3] & 0xff);
    }

    private static int readU16(byte[] b, int pos) {
        return ((b[pos] & 0xff) << 8) | (b[pos + 1] & 0xff);
    }
}
