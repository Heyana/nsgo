### Added

- End active IME compositions before reporting `OnBlur` on Android (`InputField`) and commit uncommitted preedit text before ending composition and blurring on Linux (`GtkEntry` and `GtkTextView`) without duplicating text already committed by IBus or Fcitx (#1563).
