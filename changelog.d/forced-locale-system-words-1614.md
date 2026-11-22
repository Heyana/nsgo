### Added

- Follow the language forced by `Scope.SetLocale` in system UI words (text selection menus, file dialogs and GTK widgets) in standalone apps on Android 13+ (`LocaleManager.setApplicationLocales`), Windows (`SetThreadPreferredUILanguages`/`SetProcessPreferredUILanguages`) and Linux (`LANGUAGE`/`LC_MESSAGES` and GNU gettext cache invalidation) (#1614).
