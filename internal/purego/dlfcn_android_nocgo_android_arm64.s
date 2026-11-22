//go:build android && arm64 && (android && !cgo)

#include "textflag.h"

TEXT purego_android_dlopen(SB), NOSPLIT|NOFRAME, $0-0
	JMP purego_android_dlopen_import(SB)

TEXT purego_android_dlsym(SB), NOSPLIT|NOFRAME, $0-0
	JMP purego_android_dlsym_import(SB)

TEXT purego_android_dlerror(SB), NOSPLIT|NOFRAME, $0-0
	JMP purego_android_dlerror_import(SB)

TEXT purego_android_dlclose(SB), NOSPLIT|NOFRAME, $0-0
	JMP purego_android_dlclose_import(SB)
