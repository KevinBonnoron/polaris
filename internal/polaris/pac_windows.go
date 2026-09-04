//go:build windows

package polaris

import "golang.org/x/sys/windows/registry"

// SystemPACUrl reads the PAC URL from the Windows Internet Settings registry key.
// Returns "" when absent or unreadable.
func SystemPACUrl() string {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return ""
	}
	defer k.Close()
	val, _, err := k.GetStringValue("AutoConfigURL")
	if err != nil {
		return ""
	}
	return val
}
