package nsgotest

import "image"

// GPUReadPixels returns this window's independent software GPU snapshot, as App.GPUReadPixels does for the main window.
func (s *Surface) GPUReadPixels(selector string) (*image.RGBA, error) {
	s.app.t.Helper()
	return s.open().GPUReadPixels(selector)
}

// TriggerGPUDeviceLost simulates device loss in this window's GPU view, as App.TriggerGPUDeviceLost does for the main window.
func (s *Surface) TriggerGPUDeviceLost(selector string, reason GPUDeviceLostReason, message string) {
	s.app.t.Helper()
	s.open().TriggerGPUDeviceLost(selector, reason, message)
}

// RestoreGPUDevice recreates this window's software GPU device, as App.RestoreGPUDevice does for the main window.
func (s *Surface) RestoreGPUDevice(selector string) {
	s.app.t.Helper()
	s.open().RestoreGPUDevice(selector)
}
