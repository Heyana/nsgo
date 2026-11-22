package dev.oneo.nsgo;

import android.content.Context;
import android.content.SharedPreferences;

import org.json.JSONObject;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.security.KeyPair;
import java.security.KeyPairGenerator;
import java.security.KeyStore;
import java.security.PublicKey;
import java.security.SecureRandom;
import java.security.cert.Certificate;
import java.util.Base64;
import java.util.HashMap;
import java.util.Map;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

/** Exercises Secrets.java on the JVM across native NSGo secrets and expo-secure-store migration cases. */
public final class SecretsCheck {
    private static final class MemoryKeyring implements Secrets.Keyring {
        private final SecretKey nsgo;
        private final Map<String, KeyStore.Entry> entries = new HashMap<>();

        MemoryKeyring() throws Exception {
            this.nsgo = newAESKey();
        }

        void putSecret(String alias, SecretKey key) {
            entries.put(alias, new KeyStore.SecretKeyEntry(key));
        }

        void putPrivate(String alias, KeyPair pair) {
            entries.put(alias, new KeyStore.PrivateKeyEntry(pair.getPrivate(), new Certificate[] { dummyCert(pair.getPublic()) }));
        }

        @Override
        public SecretKey nsgoKey() {
            return nsgo;
        }

        @Override
        public KeyStore.Entry entry(String alias) {
            return entries.get(alias);
        }

        @Override
        public boolean containsAlias(String alias) {
            return entries.containsKey(alias);
        }
    }

    private record Answer(int id, String error, boolean found, String value) {}

    public static void main(String[] args) throws Exception {
        if (args.length == 1 && "wire-expo".equals(args[0])) {
            emitWireExpoReply();
            return;
        }
        nativeRoundTrip();
        expoModernAES();
        expoLegacyUnprefixedAESAndIsolation();
        expoHybridRSA();
        expoErrorAndMissingKeyCases();
    }

    private static void emitWireExpoReply() throws Exception {
        File dir = Files.createTempDirectory("nsgo-secrets-wire").toFile();
        Context ctx = new Context(dir);
        MemoryKeyring keyring = new MemoryKeyring();
        SecretKey expoKey = newAESKey();
        keyring.putSecret("AES/GCM/NoPadding:key_v1:keystoreUnauthenticated", expoKey);
        SharedPreferences prefs = ctx.getSharedPreferences(Secrets.EXPO_PREFS, Context.MODE_PRIVATE);
        prefs.put("key_v1-refresh-token", makeExpoAES("wire-token-日本語", expoKey, "key_v1", true, false, false));

        byte[] payload = sendRaw(ctx, keyring, 42, Protocol.SECRET_READ, "key_v1", "refresh-token", "");
        System.out.write(payload);
        System.out.flush();
    }

    private static void nativeRoundTrip() throws Exception {
        File dir = Files.createTempDirectory("nsgo-secrets-native").toFile();
        Context ctx = new Context(dir);
        MemoryKeyring keyring = new MemoryKeyring();

        Answer miss = send(ctx, keyring, 1, Protocol.SECRET_READ, "com.example.app", "token", "");
        assertTrue(miss.error.isEmpty() && !miss.found, "missing secret should not be found: " + miss);

        Answer write = send(ctx, keyring, 2, Protocol.SECRET_WRITE, "com.example.app", "token", "hello-秘密");
        assertTrue(write.error.isEmpty(), "write failed: " + write);

        Answer hit = send(ctx, keyring, 3, Protocol.SECRET_READ, "com.example.app", "token", "");
        assertTrue(hit.error.isEmpty() && hit.found && "hello-秘密".equals(hit.value), "read mismatch: " + hit);

        Answer otherSvc = send(ctx, keyring, 4, Protocol.SECRET_READ, "com.example.other", "token", "");
        assertTrue(otherSvc.error.isEmpty() && !otherSvc.found, "wrong service read secret: " + otherSvc);

        Answer del = send(ctx, keyring, 5, Protocol.SECRET_DELETE, "com.example.app", "token", "");
        assertTrue(del.error.isEmpty(), "delete failed: " + del);

        Answer gone = send(ctx, keyring, 6, Protocol.SECRET_READ, "com.example.app", "token", "");
        assertTrue(gone.error.isEmpty() && !gone.found, "deleted secret still present: " + gone);
        assertTrue(ctx.getSharedPreferences(Secrets.EXPO_PREFS, Context.MODE_PRIVATE).editCount == 0,
                "native operations without Expo entries must not edit SecureStore prefs");
    }

    private static void expoModernAES() throws Exception {
        File dir = Files.createTempDirectory("nsgo-secrets-expo-aes").toFile();
        Context ctx = new Context(dir);
        MemoryKeyring keyring = new MemoryKeyring();
        SharedPreferences prefs = ctx.getSharedPreferences(Secrets.EXPO_PREFS, Context.MODE_PRIVATE);

        // Default keychainService ("key_v1") with usesKeystoreSuffix=true and line-wrapped Base64.
        SecretKey keyV1 = newAESKey();
        keyring.putSecret("AES/GCM/NoPadding:key_v1:keystoreUnauthenticated", keyV1);
        prefs.put("key_v1-refresh", makeExpoAES("expo-refresh-1234567890-abcdefghijklmnopqrstuvwxyz-0123456789", keyV1, "key_v1", true, false, true));

        Answer r1 = send(ctx, keyring, 10, Protocol.SECRET_READ, "key_v1", "refresh", "");
        assertTrue(r1.error.isEmpty() && r1.found && r1.value.equals("expo-refresh-1234567890-abcdefghijklmnopqrstuvwxyz-0123456789"),
                "key_v1 modern AES read failed: " + r1);

        // Custom keychainService ("com.example.app") with null requireAuthentication.
        SecretKey customKey = newAESKey();
        keyring.putSecret("AES/GCM/NoPadding:com.example.app:keystoreUnauthenticated", customKey);
        prefs.put("com.example.app-session", makeExpoAES("custom-session", customKey, "com.example.app", true, null, false));

        Answer r2 = send(ctx, keyring, 11, Protocol.SECRET_READ, "com.example.app", "session", "");
        assertTrue(r2.error.isEmpty() && r2.found && "custom-session".equals(r2.value),
                "custom service AES read failed: " + r2);

        // Writing a new value migrates to NSGo's native file and clears the Expo SharedPreferences entry.
        Answer w = send(ctx, keyring, 12, Protocol.SECRET_WRITE, "key_v1", "refresh", "nsgo-rotated-token");
        assertTrue(w.error.isEmpty(), "overwrite failed: " + w);
        assertTrue(!prefs.contains("key_v1-refresh"), "write did not remove migrated Expo entry");

        Answer r3 = send(ctx, keyring, 13, Protocol.SECRET_READ, "key_v1", "refresh", "");
        assertTrue(r3.error.isEmpty() && r3.found && "nsgo-rotated-token".equals(r3.value),
                "read after write failed: " + r3);

        // Deleting afterwards must NOT resurrect the old Expo token.
        Answer d = send(ctx, keyring, 14, Protocol.SECRET_DELETE, "key_v1", "refresh", "");
        assertTrue(d.error.isEmpty(), "delete failed: " + d);
        Answer afterDel = send(ctx, keyring, 15, Protocol.SECRET_READ, "key_v1", "refresh", "");
        assertTrue(afterDel.error.isEmpty() && !afterDel.found, "deleted secret resurrected: " + afterDel);

        // Deleting an un-migrated Expo secret removes it directly.
        Answer dCustom = send(ctx, keyring, 16, Protocol.SECRET_DELETE, "com.example.app", "session", "");
        assertTrue(dCustom.error.isEmpty() && !prefs.contains("com.example.app-session"),
                "delete did not remove Expo entry: " + dCustom);
        Answer goneCustom = send(ctx, keyring, 17, Protocol.SECRET_READ, "com.example.app", "session", "");
        assertTrue(goneCustom.error.isEmpty() && !goneCustom.found, "deleted Expo entry still read: " + goneCustom);
    }

    private static void expoLegacyUnprefixedAESAndIsolation() throws Exception {
        File dir = Files.createTempDirectory("nsgo-secrets-expo-legacy").toFile();
        Context ctx = new Context(dir);
        MemoryKeyring keyring = new MemoryKeyring();
        SharedPreferences prefs = ctx.getSharedPreferences(Secrets.EXPO_PREFS, Context.MODE_PRIVATE);

        SecretKey keyV1 = newAESKey();
        SecretKey otherKey = newAESKey();
        keyring.putSecret("AES/GCM/NoPadding:key_v1", keyV1);
        keyring.putSecret("AES/GCM/NoPadding:other.svc", otherKey);

        // Legacy unprefixed entry without keystoreAlias or usesKeystoreSuffix.
        prefs.put("legacy-token", makeExpoAES("legacy-value", keyV1, null, false, null, false));

        Answer hit = send(ctx, keyring, 20, Protocol.SECRET_READ, "key_v1", "legacy-token", "");
        assertTrue(hit.error.isEmpty() && hit.found && "legacy-value".equals(hit.value),
                "legacy unprefixed AES read failed: " + hit);

        // Partitioned TSX service without an Expo keystore key must NOT read or delete the legacy entry.
        Answer partRead = send(ctx, keyring, 21, Protocol.SECRET_READ, "com.example.app#plugin", "legacy-token", "");
        assertTrue(partRead.error.isEmpty() && !partRead.found,
                "partitioned service read legacy entry: " + partRead);
        Answer partDel = send(ctx, keyring, 22, Protocol.SECRET_DELETE, "com.example.app#plugin", "legacy-token", "");
        assertTrue(partDel.error.isEmpty() && prefs.contains("legacy-token"),
                "partitioned service deleted another service's legacy entry");

        // Another service that DOES have its own Expo key gets BadPadding -> treated as not found on legacy key.
        Answer wrongSvc = send(ctx, keyring, 23, Protocol.SECRET_READ, "other.svc", "legacy-token", "");
        assertTrue(wrongSvc.error.isEmpty() && !wrongSvc.found,
                "other service should not read or fail on key_v1 legacy entry: " + wrongSvc);

        // Keychain-aware entry takes precedence over legacy unprefixed entry.
        SecretKey modernKey = newAESKey();
        keyring.putSecret("AES/GCM/NoPadding:key_v1:keystoreUnauthenticated", modernKey);
        prefs.put("key_v1-legacy-token", makeExpoAES("newer-value", modernKey, "key_v1", true, false, false));
        Answer newer = send(ctx, keyring, 24, Protocol.SECRET_READ, "key_v1", "legacy-token", "");
        assertTrue(newer.error.isEmpty() && newer.found && "newer-value".equals(newer.value),
                "prefixed entry did not beat legacy entry: " + newer);

        // Deleting key_v1 clears both the prefixed and legacy entries.
        Answer del = send(ctx, keyring, 25, Protocol.SECRET_DELETE, "key_v1", "legacy-token", "");
        assertTrue(del.error.isEmpty() && !prefs.contains("key_v1-legacy-token") && !prefs.contains("legacy-token"),
                "delete did not clear prefixed and legacy entries");
    }

    private static void expoHybridRSA() throws Exception {
        File dir = Files.createTempDirectory("nsgo-secrets-expo-hybrid").toFile();
        Context ctx = new Context(dir);
        MemoryKeyring keyring = new MemoryKeyring();
        SharedPreferences prefs = ctx.getSharedPreferences(Secrets.EXPO_PREFS, Context.MODE_PRIVATE);

        KeyPairGenerator kpg = KeyPairGenerator.getInstance("RSA");
        kpg.initialize(2048);
        KeyPair legacyPair = kpg.generateKeyPair();
        KeyPair suffixPair = kpg.generateKeyPair();

        keyring.putPrivate("RSA/None/PKCS1Padding:key_v1", legacyPair);
        keyring.putPrivate("RSA/None/PKCS1Padding:custom.svc:keystoreUnauthenticated", suffixPair);

        prefs.put("hybrid-legacy", makeExpoHybrid("rsa-legacy-secret", legacyPair.getPublic(), null, false));
        prefs.put("custom.svc-hybrid-modern", makeExpoHybrid("rsa-modern-secret", suffixPair.getPublic(), "custom.svc", true));

        Answer r1 = send(ctx, keyring, 30, Protocol.SECRET_READ, "key_v1", "hybrid-legacy", "");
        assertTrue(r1.error.isEmpty() && r1.found && "rsa-legacy-secret".equals(r1.value),
                "legacy hybrid RSA read failed: " + r1);

        Answer r2 = send(ctx, keyring, 31, Protocol.SECRET_READ, "custom.svc", "hybrid-modern", "");
        assertTrue(r2.error.isEmpty() && r2.found && "rsa-modern-secret".equals(r2.value),
                "modern hybrid RSA read failed: " + r2);
    }

    private static void expoErrorAndMissingKeyCases() throws Exception {
        File dir = Files.createTempDirectory("nsgo-secrets-expo-err").toFile();
        Context ctx = new Context(dir);
        MemoryKeyring keyring = new MemoryKeyring();
        SharedPreferences prefs = ctx.getSharedPreferences(Secrets.EXPO_PREFS, Context.MODE_PRIVATE);

        SecretKey key = newAESKey();
        // 1. Missing KeyStore alias (e.g., reinstalled app after Auto Backup restored SharedPreferences) -> not found.
        prefs.put("reinstalled-tok", makeExpoAES("lost", key, "reinstalled", true, false, false));
        Answer missingKey = send(ctx, keyring, 40, Protocol.SECRET_READ, "reinstalled", "tok", "");
        assertTrue(missingKey.error.isEmpty() && !missingKey.found,
                "missing keystore alias should return not found: " + missingKey);

        // Register key for remaining error checks.
        keyring.putSecret("AES/GCM/NoPadding:key_v1:keystoreUnauthenticated", key);

        // 2. Authentication-gated items report a clear error.
        for (Object auth : new Object[] { Boolean.TRUE, "biometry", "deviceCredentials" }) {
            prefs.put("key_v1-auth", makeExpoAES("locked", key, "key_v1", true, auth, false));
            Answer a = send(ctx, keyring, 41, Protocol.SECRET_READ, "key_v1", "auth", "");
            assertTrue(!a.found && a.error.contains("user authentication"),
                    "auth=" + auth + " should fail with authentication error: " + a);
        }

        // 3. Tag length below 96 bits is rejected.
        JSONObject shortTag = new JSONObject(makeExpoAES("short", key, "key_v1", true, false, false));
        shortTag.put("tlen", 64);
        prefs.put("key_v1-short", shortTag.toString());
        Answer badTagLen = send(ctx, keyring, 42, Protocol.SECRET_READ, "key_v1", "short", "");
        assertTrue(!badTagLen.found && badTagLen.error.contains("below 96"),
                "short tag length should be rejected: " + badTagLen);

        // 4. Unknown scheme is rejected on prefixed entry.
        JSONObject unknownScheme = new JSONObject()
                .put("scheme", "xor")
                .put("keystoreAlias", "key_v1")
                .put("usesKeystoreSuffix", true);
        prefs.put("key_v1-xor", unknownScheme.toString());
        Answer badScheme = send(ctx, keyring, 43, Protocol.SECRET_READ, "key_v1", "xor", "");
        assertTrue(!badScheme.found && badScheme.error.contains("unknown expo-secure-store scheme"),
                "unknown scheme should be rejected: " + badScheme);

        // 5. Tampered ciphertext on prefixed entry is rejected.
        JSONObject tampered = new JSONObject(makeExpoAES("authentic", key, "key_v1", true, false, false));
        byte[] ct = Base64.getDecoder().decode(tampered.getString("ct"));
        ct[0] ^= 0x01;
        tampered.put("ct", Base64.getEncoder().encodeToString(ct));
        prefs.put("key_v1-tampered", tampered.toString());
        Answer badCT = send(ctx, keyring, 44, Protocol.SECRET_READ, "key_v1", "tampered", "");
        assertTrue(!badCT.found && !badCT.error.isEmpty(),
                "tampered ciphertext should fail: " + badCT);
    }

    private static SecretKey newAESKey() throws Exception {
        KeyGenerator gen = KeyGenerator.getInstance("AES");
        gen.init(256);
        return gen.generateKey();
    }

    private static String makeExpoAES(String plaintext, SecretKey key, String keystoreAlias, boolean usesSuffix, Object requireAuth, boolean wrapLines) throws Exception {
        byte[] iv = new byte[12];
        new SecureRandom().nextBytes(iv);
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.ENCRYPT_MODE, key, new GCMParameterSpec(128, iv));
        byte[] ct = cipher.doFinal(plaintext.getBytes(StandardCharsets.UTF_8));
        Base64.Encoder enc = wrapLines ? Base64.getMimeEncoder(16, new byte[] { '\n' }) : Base64.getEncoder();
        JSONObject obj = new JSONObject()
                .put("ct", enc.encodeToString(ct))
                .put("iv", enc.encodeToString(iv))
                .put("tlen", 128)
                .put("scheme", "aes");
        if (usesSuffix) obj.put("usesKeystoreSuffix", true);
        if (keystoreAlias != null) obj.put("keystoreAlias", keystoreAlias);
        if (requireAuth != null) obj.put("requireAuthentication", requireAuth);
        return obj.toString();
    }

    private static String makeExpoHybrid(String plaintext, PublicKey rsaPublic, String keystoreAlias, boolean usesSuffix) throws Exception {
        SecretKey aesKey = newAESKey();
        Cipher rsa = Cipher.getInstance("RSA/ECB/PKCS1Padding");
        rsa.init(Cipher.ENCRYPT_MODE, rsaPublic);
        byte[] esk = rsa.doFinal(aesKey.getEncoded());

        byte[] iv = new byte[12];
        new SecureRandom().nextBytes(iv);
        Cipher aes = Cipher.getInstance("AES/GCM/NoPadding");
        aes.init(Cipher.ENCRYPT_MODE, aesKey, new GCMParameterSpec(128, iv));
        byte[] ct = aes.doFinal(plaintext.getBytes(StandardCharsets.UTF_8));

        JSONObject obj = new JSONObject()
                .put("ct", Base64.getEncoder().encodeToString(ct))
                .put("iv", Base64.getEncoder().encodeToString(iv))
                .put("tlen", 128)
                .put("esk", Base64.getEncoder().encodeToString(esk))
                .put("scheme", "hybrid");
        if (usesSuffix) obj.put("usesKeystoreSuffix", true);
        if (keystoreAlias != null) obj.put("keystoreAlias", keystoreAlias);
        return obj.toString();
    }

    private static byte[] sendRaw(Context ctx, Secrets.Keyring keyring, int id, int op, String service, String account, String value) throws Exception {
        ByteArrayOutputStream req = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(req);
        out.writeInt(id);
        out.writeByte(op);
        Protocol.writeString(out, service);
        Protocol.writeString(out, account);
        Protocol.writeString(out, value);
        out.flush();
        return Secrets.handle(ctx, new DataInputStream(new ByteArrayInputStream(req.toByteArray())), keyring);
    }

    private static Answer send(Context ctx, Secrets.Keyring keyring, int id, int op, String service, String account, String value) throws Exception {
        byte[] raw = sendRaw(ctx, keyring, id, op, service, account, value);
        DataInputStream in = new DataInputStream(new ByteArrayInputStream(raw));
        int gotId = in.readInt();
        byte[] errBytes = new byte[in.readInt()];
        in.readFully(errBytes);
        boolean found = in.readUnsignedByte() != 0;
        byte[] valBytes = new byte[in.readInt()];
        in.readFully(valBytes);
        assertTrue(gotId == id, "reply id " + gotId + " != " + id);
        return new Answer(gotId, new String(errBytes, StandardCharsets.UTF_8), found, new String(valBytes, StandardCharsets.UTF_8));
    }

    private static Certificate dummyCert(PublicKey pub) {
        return new Certificate("X.509") {
            @Override public byte[] getEncoded() { return new byte[0]; }
            @Override public void verify(PublicKey key) {}
            @Override public void verify(PublicKey key, String sigProvider) {}
            @Override public String toString() { return "dummy"; }
            @Override public PublicKey getPublicKey() { return pub; }
        };
    }

    private static void assertTrue(boolean cond, String message) {
        if (!cond) throw new AssertionError(message);
    }
}
