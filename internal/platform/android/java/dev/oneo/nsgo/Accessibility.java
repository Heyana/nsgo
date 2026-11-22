package dev.oneo.nsgo;

/** Maps NSGo's accessibility roles and states to Android's AccessibilityNodeInfo class names, localized role descriptions, checkable states and state descriptions. Pure Java so JVM tests can verify every role and state against Go's protocol and locale catalog without an Android runtime. */
final class Accessibility {
    static final class Description {
        final String className;
        final String roleDescription;
        final boolean heading;
        final boolean checkable;
        final boolean checked;
        final String stateDescription;

        Description(String className, String roleDescription, boolean heading,
                boolean checkable, boolean checked, String stateDescription) {
            this.className = className;
            this.roleDescription = roleDescription;
            this.heading = heading;
            this.checkable = checkable;
            this.checked = checked;
            this.stateDescription = stateDescription;
        }
    }

    private Accessibility() {}

    /** Returns the word Go sent for Protocol.SPOKEN_* index i, or null if unavailable. */
    static String spokenWord(String[] spoken, int index) {
        if (spoken == null || index < 0 || index >= spoken.length) return null;
        String word = spoken[index];
        return word == null || word.isEmpty() ? null : word;
    }

    /** Appends part, unless null or empty, to TalkBack's state description with the localized separator. */
    static void appendState(StringBuilder state, String part, String[] spoken) {
        if (part == null || part.isEmpty()) return;
        if (state.length() > 0) {
            String separator = spokenWord(spoken, Protocol.SPOKEN_SEPARATOR);
            state.append(separator != null ? separator : ", ");
        }
        state.append(part);
    }

    /** Returns the localized level phrase for Protocol.SPOKEN_* index i with level substituted, or null if unavailable. */
    static String spokenLevel(String[] spoken, int index, int level) {
        String pattern = spokenWord(spoken, index);
        return pattern != null ? pattern.replace("{level}", Integer.toString(level)) : null;
    }

    /** Computes the Android accessibility class, role description, check state and state description for a node; fieldText is the text of the node's input, or null, which valueText is not repeated as, and level is View.AccessibilityLevel (0 for none). */
    static Description describe(int role, boolean isField, String fieldText, boolean checked, boolean mixed,
            boolean pressed, boolean busy, boolean expandable, boolean expanded, int level,
            String valueText, String[] spoken) {
        String className = null;
        int spokenRole = -1;
        boolean heading = false;
        boolean checkable = false;
        switch (role) {
            case Protocol.ROLE_BUTTON: className = "android.widget.Button"; break;
            case Protocol.ROLE_LINK: spokenRole = Protocol.SPOKEN_LINK; break;
            case Protocol.ROLE_HEADER: heading = true; break;
            case Protocol.ROLE_IMAGE: className = "android.widget.ImageView"; break;
            case Protocol.ROLE_CHECKBOX: className = "android.widget.CheckBox"; checkable = true; break;
            case Protocol.ROLE_SWITCH: className = "android.widget.Switch"; checkable = true; break;
            case Protocol.ROLE_RADIO: className = "android.widget.RadioButton"; checkable = true; break;
            case Protocol.ROLE_TAB: spokenRole = Protocol.SPOKEN_TAB; break;
            case Protocol.ROLE_TEXT: className = "android.widget.TextView"; break;
            case Protocol.ROLE_TEXT_FIELD: className = "android.widget.EditText"; break;
            // An edit text still, which TalkBack edits as one, named a search box.
            case Protocol.ROLE_SEARCH_FIELD: className = "android.widget.EditText"; spokenRole = Protocol.SPOKEN_SEARCH_BOX; break;
            // An edit text when mounted on an input, which TalkBack edits as one, named a spin button.
            case Protocol.ROLE_SPIN_BUTTON:
                if (isField) className = "android.widget.EditText";
                spokenRole = Protocol.SPOKEN_SPIN_BUTTON;
                break;
            case Protocol.ROLE_STATUS: spokenRole = Protocol.SPOKEN_STATUS; break;
            case Protocol.ROLE_RADIO_GROUP: className = "android.widget.RadioGroup"; break;
            case Protocol.ROLE_TAB_LIST: className = "android.widget.TabWidget"; break;
            case Protocol.ROLE_MENU: spokenRole = Protocol.SPOKEN_MENU; break;
            case Protocol.ROLE_MENU_ITEM: spokenRole = Protocol.SPOKEN_MENU_ITEM; break;
            case Protocol.ROLE_MENU_ITEM_CHECKBOX:
                className = "android.widget.CheckBox";
                spokenRole = Protocol.SPOKEN_MENU_ITEM;
                checkable = true;
                break;
            case Protocol.ROLE_MENU_ITEM_RADIO:
                className = "android.widget.RadioButton";
                spokenRole = Protocol.SPOKEN_MENU_ITEM;
                checkable = true;
                break;
            case Protocol.ROLE_DIALOG: spokenRole = Protocol.SPOKEN_DIALOG; break;
            case Protocol.ROLE_ALERT: spokenRole = Protocol.SPOKEN_ALERT; break;
            case Protocol.ROLE_ADJUSTABLE: className = "android.widget.SeekBar"; break;
            case Protocol.ROLE_PROGRESS_BAR: className = "android.widget.ProgressBar"; break;
            case Protocol.ROLE_TABLE: spokenRole = Protocol.SPOKEN_TABLE; break;
            case Protocol.ROLE_GRID: spokenRole = Protocol.SPOKEN_GRID; break;
            case Protocol.ROLE_ROW: spokenRole = Protocol.SPOKEN_ROW; break;
            case Protocol.ROLE_COLUMN_HEADER: spokenRole = Protocol.SPOKEN_COLUMN_HEADER; heading = true; break;
            case Protocol.ROLE_CELL: spokenRole = Protocol.SPOKEN_CELL; break;
            case Protocol.ROLE_LISTBOX: spokenRole = Protocol.SPOKEN_LIST_BOX; break;
            case Protocol.ROLE_OPTION: spokenRole = Protocol.SPOKEN_OPTION; break;
            // Editable comboboxes stay EditText so TalkBack edits them as text fields.
            case Protocol.ROLE_COMBOBOX:
                if (isField) className = "android.widget.EditText";
                spokenRole = Protocol.SPOKEN_COMBO_BOX;
                break;
            case Protocol.ROLE_GROUP: spokenRole = Protocol.SPOKEN_GROUP; break;
            case Protocol.ROLE_REGION: spokenRole = Protocol.SPOKEN_REGION; break;
            case Protocol.ROLE_NAVIGATION: spokenRole = Protocol.SPOKEN_NAVIGATION; break;
            case Protocol.ROLE_TREE: spokenRole = Protocol.SPOKEN_TREE; break;
            case Protocol.ROLE_TREE_ITEM: spokenRole = Protocol.SPOKEN_TREE_ITEM; break;
            case Protocol.ROLE_TOOLBAR: spokenRole = Protocol.SPOKEN_TOOLBAR; break;
            default: break;
        }
        String roleDescription = role == Protocol.ROLE_HEADER && level > 0
                ? spokenLevel(spoken, Protocol.SPOKEN_HEADING_LEVEL, level) : spokenWord(spoken, spokenRole);
        StringBuilder state = new StringBuilder();
        boolean isChecked = checkable && checked && !mixed;
        if (checkable && mixed) {
            // The checked state has no mixed value before API 36, so describe it.
            appendState(state, spokenWord(spoken, Protocol.SPOKEN_PARTIALLY_CHECKED), spoken);
        }
        if (pressed) {
            appendState(state, spokenWord(spoken, Protocol.SPOKEN_PRESSED), spoken);
        }
        if (level > 0 && role != Protocol.ROLE_HEADER) {
            appendState(state, spokenLevel(spoken, Protocol.SPOKEN_LEVEL, level), spoken);
        }
        if (expandable) {
            appendState(state, spokenWord(spoken, expanded ? Protocol.SPOKEN_EXPANDED : Protocol.SPOKEN_COLLAPSED), spoken);
        }
        if (busy) {
            appendState(state, spokenWord(spoken, Protocol.SPOKEN_BUSY), spoken);
        }
        if (valueText != null && !valueText.isEmpty() && !valueText.equals(fieldText)) {
            appendState(state, valueText, spoken);
        }
        String stateDescription = state.length() > 0 ? state.toString() : null;
        return new Description(className, roleDescription, heading, checkable, isChecked, stateDescription);
    }
}
