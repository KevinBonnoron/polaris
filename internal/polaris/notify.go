package polaris

import (
	"log"

	"github.com/gen2brain/beeep"
)

// dispatchOSNotification surfaces a native toast for n, picking between the
// silent variant and the sound-enabled one based on the user's settings.
// Errors are swallowed (logged only): notification failures must never bubble
// up to break the producer that triggered the event.
func dispatchOSNotification(n Notification, settings NotificationSettings) {
	if n.Title == "" {
		return
	}
	const title = "Polaris"
	var err error
	if settings.Sound == "" || settings.Sound == "none" {
		err = beeep.Notify(title, n.Title, "")
	} else {
		err = beeep.Alert(title, n.Title, "")
	}
	if err != nil {
		log.Printf("polaris: os notification failed: %v", err)
	}
}
