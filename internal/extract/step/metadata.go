package step

import "fmt"

const KeyPersonalData = "classification.personal_data"

func validateMetadata(metadata map[string]any) error {
	value, ok := metadata[KeyPersonalData]
	if !ok {
		return fmt.Errorf("%w: %s is missing, set it in the source attributes or in the sidecar", ErrInvalidMetadata, KeyPersonalData)
	}
	if _, ok := value.(bool); !ok {
		return fmt.Errorf("%w: %s must be true or false, got %v (%T)", ErrInvalidMetadata, KeyPersonalData, value, value)
	}
	return nil
}
