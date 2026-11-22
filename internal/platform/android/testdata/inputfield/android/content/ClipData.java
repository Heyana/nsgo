package android.content;

import android.net.Uri;

public class ClipData {
    public static class Item {
        public Uri getUri() { return null; }
        public CharSequence getText() { return null; }
    }
    public int getItemCount() { return 0; }
    public Item getItemAt(int index) { return new Item(); }
}
