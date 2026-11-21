package android.view;

public interface Menu {
    int NONE = 0;

    MenuItem add(int group, int id, int order, int title);
}
