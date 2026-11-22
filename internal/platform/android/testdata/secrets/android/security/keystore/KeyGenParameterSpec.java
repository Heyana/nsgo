package android.security.keystore;

import java.security.spec.AlgorithmParameterSpec;

/** Compile-time stub for Android KeyGenParameterSpec on the host JVM. */
public final class KeyGenParameterSpec implements AlgorithmParameterSpec {
    public static final class Builder {
        public Builder(String keystoreAlias, int purposes) {}
        public Builder setBlockModes(String... blockModes) { return this; }
        public Builder setEncryptionPaddings(String... paddings) { return this; }
        public Builder setKeySize(int keySize) { return this; }
        public KeyGenParameterSpec build() { return new KeyGenParameterSpec(); }
    }
}
