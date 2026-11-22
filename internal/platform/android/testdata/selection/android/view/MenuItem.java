package android.view;

public interface MenuItem {
    int SHOW_AS_ACTION_IF_ROOM = 1;

    int getItemId();

    int getGroupId();

    int getOrder();

    void setShowAsAction(int actionEnum);
}
