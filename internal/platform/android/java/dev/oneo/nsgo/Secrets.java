package dev.oneo.nsgo;

import android.content.Context;
import android.content.SharedPreferences;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.AtomicFile;

import org.json.JSONException;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.KeyStore;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.Base64;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import javax.crypto.BadPaddingException;
import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;

/**
 * The app's secrets (secrets.go): each value encrypted with AES-GCM under one key of the app's in AndroidKeyStore, which never leaves the system's keystore, its service and account as the associated data, and kept in a file of the app's no-backup files named after their SHA-256, which neither other apps nor backups reach. Reads also fall back to values expo-secure-store encrypted into its SecureStore SharedPreferences (AES-GCM and legacy hybrid RSA+AES-GCM), so a store build moving from Expo keeps its users signed in; writes and deletes clear any matching expo-secure-store entry. One thread serves the requests in order, off the reader thread.
 */
final class Secrets {
    private Secrets() {}

    /** The key's alias in AndroidKeyStore. */
    private static final String KEY = "dev.oneo.nsgo.secrets";
    /** The directory of the ciphertexts in the app's no-backup files. */
    private static final String DIR = "nsgo-secrets";
    /** The first byte of a file: its format, then the IV's length, the IV and the ciphertext with its tag. */
    private static final int FORMAT = 1;
    private static final int TAG_BITS = 128;

    /** expo-secure-store's SharedPreferences file, JSON fields and keystore alias parts. */
    static final String EXPO_PREFS = "SecureStore";
    private static final String EXPO_SCHEME = "scheme";
    private static final String EXPO_SCHEME_AES = "aes";
    private static final String EXPO_SCHEME_HYBRID = "hybrid";
    private static final String EXPO_CIPHERTEXT = "ct";
    private static final String EXPO_IV = "iv";
    private static final String EXPO_TLEN = "tlen";
    private static final String EXPO_ENCRYPTED_KEY = "esk";
    private static final String EXPO_KEYSTORE_ALIAS = "keystoreAlias";
    private static final String EXPO_USES_KEYSTORE_SUFFIX = "usesKeystoreSuffix";
    private static final String EXPO_REQUIRE_AUTH = "requireAuthentication";
    private static final String EXPO_UNAUTHENTICATED_SUFFIX = "keystoreUnauthenticated";
    private static final String EXPO_AES_CIPHER = "AES/GCM/NoPadding";
    private static final String EXPO_RSA_CIPHER = "RSA/None/PKCS1Padding";
    private static final int EXPO_MIN_TAG_BITS = 96;

    private static final ExecutorService worker = Executors.newSingleThreadExecutor();

    /** Keystore access, overridable by JVM tests without AndroidKeyStore. */
    interface Keyring {
        SecretKey nsgoKey() throws GeneralSecurityException, IOException;
        KeyStore.Entry entry(String alias) throws GeneralSecurityException, IOException;
        boolean containsAlias(String alias) throws GeneralSecurityException, IOException;
    }

    private static final Keyring ANDROID_KEYRING = new Keyring() {
        @Override
        public SecretKey nsgoKey() throws GeneralSecurityException, IOException {
            KeyStore store = androidKeyStore();
            if (store.getKey(KEY, null) instanceof SecretKey key) return key;
            KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
            generator.init(new KeyGenParameterSpec.Builder(KEY, KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setKeySize(256)
                    .build());
            return generator.generateKey();
        }

        @Override
        public KeyStore.Entry entry(String alias) throws GeneralSecurityException, IOException {
            KeyStore store = androidKeyStore();
            if (!store.containsAlias(alias)) return null;
            return store.getEntry(alias, null);
        }

        @Override
        public boolean containsAlias(String alias) throws GeneralSecurityException, IOException {
            return androidKeyStore().containsAlias(alias);
        }
    };

    /** Sends an answer to Go. */
    interface Reply {
        void send(byte[] payload);
    }

    /** Answers a SECRET message on the worker: its number, an error, whether a value was found and the value. */
    static void answer(Context context, DataInputStream in, Reply reply) {
        worker.execute(() -> reply.send(handle(context, in, ANDROID_KEYRING)));
    }

    static byte[] handle(Context context, DataInputStream in, Keyring keyring) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(bytes);
        try {
            int id = in.readInt();
            int op = in.readUnsignedByte();
            byte[] service = read(in), account = read(in), value = read(in);
            String serviceName = new String(service, StandardCharsets.UTF_8);
            String accountName = new String(account, StandardCharsets.UTF_8);
            String error = "";
            boolean found = false;
            byte[] result = new byte[0];
            try {
                File file = file(context, service, account);
                byte[] aad = associated(service, account);
                switch (op) {
                    case Protocol.SECRET_READ:
                        if (file.exists()) {
                            result = decrypt(new AtomicFile(file).readFully(), aad, keyring);
                            found = true;
                        } else {
                            byte[] expo = readExpo(context, serviceName, accountName, keyring);
                            if (expo != null) {
                                result = expo;
                                found = true;
                            }
                        }
                        break;
                    case Protocol.SECRET_WRITE:
                        write(file, encrypt(value, aad, keyring));
                        removeExpo(context, serviceName, accountName, keyring);
                        break;
                    case Protocol.SECRET_DELETE:
                        new AtomicFile(file).delete();
                        removeExpo(context, serviceName, accountName, keyring);
                        break;
                    default:
                        error = "no secret request " + op;
                }
            } catch (IOException | GeneralSecurityException | JSONException | RuntimeException e) {
                error = e.getClass().getSimpleName() + ": " + e.getMessage();
                found = false;
                result = new byte[0];
            }
            out.writeInt(id);
            Protocol.writeString(out, error);
            out.writeByte(found ? 1 : 0);
            out.writeInt(result.length);
            out.write(result);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        return bytes.toByteArray();
    }

    private static byte[] read(DataInputStream in) throws IOException {
        byte[] b = new byte[in.readInt()];
        in.readFully(b);
        return b;
    }

    /** The service's length in four bytes, the service and the account, so no other service and account make the same bytes. */
    private static byte[] associated(byte[] service, byte[] account) {
        byte[] b = new byte[4 + service.length + account.length];
        int n = service.length;
        b[0] = (byte) (n >>> 24);
        b[1] = (byte) (n >>> 16);
        b[2] = (byte) (n >>> 8);
        b[3] = (byte) n;
        System.arraycopy(service, 0, b, 4, n);
        System.arraycopy(account, 0, b, 4 + n, account.length);
        return b;
    }

    private static File file(Context context, byte[] service, byte[] account) throws GeneralSecurityException {
        byte[] digest = MessageDigest.getInstance("SHA-256").digest(associated(service, account));
        StringBuilder name = new StringBuilder(digest.length * 2);
        for (byte b : digest) name.append(String.format("%02x", b));
        return new File(new File(context.getNoBackupFilesDir(), DIR), name.toString());
    }

    private static void write(File file, byte[] data) throws IOException {
        File dir = file.getParentFile();
        if (!dir.isDirectory() && !dir.mkdirs()) throw new IOException("cannot make " + dir);
        AtomicFile atomic = new AtomicFile(file);
        FileOutputStream stream = atomic.startWrite();
        try {
            stream.write(data);
            atomic.finishWrite(stream);
        } catch (IOException e) {
            atomic.failWrite(stream);
            throw e;
        }
    }

    private static KeyStore androidKeyStore() throws GeneralSecurityException, IOException {
        KeyStore store = KeyStore.getInstance("AndroidKeyStore");
        store.load(null);
        return store;
    }

    private static byte[] encrypt(byte[] value, byte[] aad, Keyring keyring) throws GeneralSecurityException, IOException {
        Cipher cipher = Cipher.getInstance(EXPO_AES_CIPHER);
        // The keystore picks the IV, as it refuses one the caller gives.
        cipher.init(Cipher.ENCRYPT_MODE, keyring.nsgoKey());
        cipher.updateAAD(aad);
        byte[] sealed = cipher.doFinal(value);
        byte[] iv = cipher.getIV();
        ByteArrayOutputStream out = new ByteArrayOutputStream(2 + iv.length + sealed.length);
        out.write(FORMAT);
        out.write(iv.length);
        out.write(iv);
        out.write(sealed);
        return out.toByteArray();
    }

    private static byte[] decrypt(byte[] data, byte[] aad, Keyring keyring) throws GeneralSecurityException, IOException {
        if (data.length < 2 || data[0] != FORMAT || data.length < 2 + (data[1] & 0xff)) {
            throw new IOException("the secret's file is not one this runtime wrote");
        }
        int ivLength = data[1] & 0xff;
        Cipher cipher = Cipher.getInstance(EXPO_AES_CIPHER);
        cipher.init(Cipher.DECRYPT_MODE, keyring.nsgoKey(), new GCMParameterSpec(TAG_BITS, data, 2, ivLength));
        cipher.updateAAD(aad);
        return cipher.doFinal(data, 2 + ivLength, data.length - 2 - ivLength);
    }

    /**
     * Reads an item expo-secure-store encrypted into its SecureStore SharedPreferences, under "<service>-<account>" or the legacy unprefixed "<account>". Returns null when no item or keystore entry exists for service and account.
     */
    private static byte[] readExpo(Context context, String service, String account, Keyring keyring)
            throws GeneralSecurityException, IOException, JSONException {
        SharedPreferences prefs = context.getSharedPreferences(EXPO_PREFS, Context.MODE_PRIVATE);
        String keychainKey = service + "-" + account;
        String raw = prefs.getString(keychainKey, null);
        boolean legacy = false;
        if (raw == null) {
            raw = prefs.getString(account, null);
            legacy = true;
        }
        if (raw == null) return null;

        JSONObject item;
        try {
            item = new JSONObject(raw);
        } catch (JSONException e) {
            if (legacy) return null;
            throw e;
        }
        String storedAlias = item.optString(EXPO_KEYSTORE_ALIAS, "");
        if (!storedAlias.isEmpty() && !storedAlias.equals(service)) {
            return null;
        }
        Object auth = item.opt(EXPO_REQUIRE_AUTH);
        if (auth != null && auth != JSONObject.NULL && !Boolean.FALSE.equals(auth) && !"none".equals(auth)) {
            if (Boolean.TRUE.equals(auth) || "biometry".equals(auth) || "deviceCredentials".equals(auth)) {
                throw new GeneralSecurityException("expo-secure-store item requires user authentication");
            }
            throw new GeneralSecurityException("unknown expo-secure-store requireAuthentication value");
        }

        String scheme = item.optString(EXPO_SCHEME, "");
        boolean usesSuffix = item.optBoolean(EXPO_USES_KEYSTORE_SUFFIX, false);
        try {
            if (EXPO_SCHEME_AES.equals(scheme)) {
                String alias = expoKeystoreAlias(EXPO_AES_CIPHER, service, usesSuffix);
                KeyStore.Entry entry = keyring.entry(alias);
                if (entry == null) return null;
                if (!(entry instanceof KeyStore.SecretKeyEntry secretEntry)) {
                    throw new GeneralSecurityException("keystore entry " + alias + " is not a secret key");
                }
                return decryptExpoAES(item, secretEntry.getSecretKey());
            }
            if (EXPO_SCHEME_HYBRID.equals(scheme)) {
                String alias = expoKeystoreAlias(EXPO_RSA_CIPHER, service, usesSuffix);
                KeyStore.Entry entry = keyring.entry(alias);
                if (entry == null) return null;
                if (!(entry instanceof KeyStore.PrivateKeyEntry privateEntry)) {
                    throw new GeneralSecurityException("keystore entry " + alias + " is not a private key");
                }
                byte[] wrappedKey = decodeBase64(item.getString(EXPO_ENCRYPTED_KEY));
                Cipher rsa = rsaCipher();
                rsa.init(Cipher.DECRYPT_MODE, privateEntry.getPrivateKey());
                byte[] aesKeyBytes = rsa.doFinal(wrappedKey);
                return decryptExpoAES(item, new SecretKeySpec(aesKeyBytes, "AES"));
            }
            if (legacy) return null;
            throw new GeneralSecurityException("unknown expo-secure-store scheme " + scheme);
        } catch (BadPaddingException e) {
            // A legacy unprefixed entry may have been encrypted under another keychainService's key.
            if (legacy) return null;
            throw e;
        }
    }

    /** Removes any expo-secure-store SharedPreferences entry owned by service and account. */
    private static void removeExpo(Context context, String service, String account, Keyring keyring)
            throws GeneralSecurityException, IOException {
        SharedPreferences prefs = context.getSharedPreferences(EXPO_PREFS, Context.MODE_PRIVATE);
        String keychainKey = service + "-" + account;
        boolean removeAware = prefs.contains(keychainKey);
        boolean removeLegacy = false;
        if (prefs.contains(account)) {
            String raw = prefs.getString(account, null);
            if (raw != null) {
                try {
                    JSONObject item = new JSONObject(raw);
                    String storedAlias = item.optString(EXPO_KEYSTORE_ALIAS, "");
                    if (!storedAlias.isEmpty()) {
                        removeLegacy = storedAlias.equals(service);
                    } else {
                        String scheme = item.optString(EXPO_SCHEME, "");
                        boolean usesSuffix = item.optBoolean(EXPO_USES_KEYSTORE_SUFFIX, false);
                        if (EXPO_SCHEME_AES.equals(scheme)) {
                            removeLegacy = keyring.containsAlias(expoKeystoreAlias(EXPO_AES_CIPHER, service, usesSuffix));
                        } else if (EXPO_SCHEME_HYBRID.equals(scheme)) {
                            removeLegacy = keyring.containsAlias(expoKeystoreAlias(EXPO_RSA_CIPHER, service, usesSuffix));
                        }
                    }
                } catch (JSONException ignored) {
                }
            }
        }
        if (!removeAware && !removeLegacy) return;
        SharedPreferences.Editor editor = prefs.edit();
        if (removeAware) editor.remove(keychainKey);
        if (removeLegacy) editor.remove(account);
        editor.commit();
    }

    private static String expoKeystoreAlias(String cipher, String service, boolean usesSuffix) {
        String base = cipher + ":" + service;
        return usesSuffix ? base + ":" + EXPO_UNAUTHENTICATED_SUFFIX : base;
    }

    private static byte[] decryptExpoAES(JSONObject item, SecretKey key) throws GeneralSecurityException, JSONException {
        byte[] ct = decodeBase64(item.getString(EXPO_CIPHERTEXT));
        byte[] iv = decodeBase64(item.getString(EXPO_IV));
        int tlen = item.getInt(EXPO_TLEN);
        if (tlen < EXPO_MIN_TAG_BITS) {
            throw new GeneralSecurityException("GCM tag length " + tlen + " is below " + EXPO_MIN_TAG_BITS);
        }
        Cipher cipher = Cipher.getInstance(EXPO_AES_CIPHER);
        cipher.init(Cipher.DECRYPT_MODE, key, new GCMParameterSpec(tlen, iv));
        return cipher.doFinal(ct);
    }

    private static Cipher rsaCipher() throws GeneralSecurityException {
        try {
            return Cipher.getInstance(EXPO_RSA_CIPHER);
        } catch (NoSuchAlgorithmException e) {
            // Standard JVMs name the same single-block PKCS#1 v1.5 transform RSA/ECB/PKCS1Padding.
            return Cipher.getInstance("RSA/ECB/PKCS1Padding");
        }
    }

    private static byte[] decodeBase64(String value) {
        return Base64.getDecoder().decode(value.replaceAll("\\s+", ""));
    }
}
