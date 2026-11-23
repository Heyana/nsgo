package android

import (
	"encoding/json"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func pictureInPictureCommandJSON(command, token uint32) string {
	data, _ := json.Marshal(struct {
		Command uint32 `json:"command"`
		Token   uint32 `json:"token"`
	}{command, token})
	return string(data)
}

func readPictureInPictureEvent(data string) (*driver.PictureInPictureEvent, bool) {
	var event driver.PictureInPictureEvent
	err := json.Unmarshal([]byte(data), &event)
	return &event, err == nil
}
