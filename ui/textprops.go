package ui

// textValue reads a text prop, with its function taking precedence.
func textValue(text string, read func() string) string {
	if read != nil {
		return read()
	}
	return text
}

// textBinding supplies the fallback when a control has no explicit name.
func textBinding(text string, read func() string, fallback string, readFallback func() string) (string, func() string) {
	switch {
	case read != nil:
		return "", func() string {
			if label := read(); label != "" {
				return label
			}
			return textValue(fallback, readFallback)
		}
	case text != "":
		return text, nil
	case readFallback != nil:
		return "", readFallback
	default:
		return fallback, nil
	}
}
