### Added

- Update `time.Local` live when the system time zone changes on Windows (`WM_TIMECHANGE`/`WM_SETTINGCHANGE`, `GetDynamicTimeZoneInformation` and `icu.dll`/CLDR mapping with DST-aware TZif synthesis), Linux (`inotify` on `/etc/localtime` and `/etc/timezone` plus `org.freedesktop.timedate1` `Timezone` changes) and the web (`Intl.DateTimeFormat().resolvedOptions().timeZone` and `Date` offset checks on focus, visibility, `pageshow`, `languagechange` and a visible-only timer) (#1539).
