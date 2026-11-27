### Fixed

- Allocate resized Linux GTK child views synchronously inside `size_allocate` instead of re-queueing `gtk_widget_queue_allocate` onto the next frame clock cycle (#1375 by @houko).
