package android.content.res;

public class Resources {
    public Configuration getConfiguration() {
        return new Configuration();
    }

    public static class Theme {
        public void applyStyle(int resId, boolean force) {}
    }
}
