### Fixed

- Fix and verify `View.OnWheel`, pinch `Position` across all gesture phases, and Android touchpad scroll momentum glide across iOS (fixing swapped `UIScrollTypeMask` continuous/discrete masks and scroll-view precedence), Android (`WheelScroll` release-velocity tracking and transformed pinch unprojection), Windows, Linux (`gtk_gesture_get_bounding_box_center` and touchpad phases) and the web (#1607).
