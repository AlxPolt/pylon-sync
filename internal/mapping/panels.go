package mapping

import "fmt"

// FormatPanels produces the "{qty}x{watt}W" string.
// dcOutputKW is the system output in kilowatts; quantity is the number of panels.
// Example: 9.5 kW / 20 panels = 475 W → "20x475W"
func FormatPanels(dcOutputKW float64, quantity int) string {
	if quantity <= 0 {
		return ""
	}
	wattPerPanel := int(dcOutputKW * 1000 / float64(quantity))
	return fmt.Sprintf("%dx%dW", quantity, wattPerPanel)
}
