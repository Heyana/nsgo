package android.view;

public interface Menu {
    int NONE = 0;
    MenuItem add(int groupId, int itemId, int order, int titleRes);
    MenuItem findItem(int id);
}
