//go:build linux

package main

/*
#include <stdlib.h>
#include <string.h>

// Runs before Go runtime starts. Forces XWayland because WebKitGTK's
// native Wayland surface mis-sizes the webview on multi-monitor ultra-wide
// setups. Honors an explicit non-wayland GDK_BACKEND from the user.
__attribute__((constructor(101))) static void polaris_set_linux_env(void) {
    if (getenv("WAYLAND_DISPLAY") == NULL) return;
    const char *gdk = getenv("GDK_BACKEND");
    if (gdk == NULL || strstr(gdk, "wayland") != NULL) {
        setenv("GDK_BACKEND", "x11", 1);
    }
}
*/
import "C"
