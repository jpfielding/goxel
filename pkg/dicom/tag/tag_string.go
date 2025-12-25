package tag

import (
	"encoding/json"
	"fmt"
)

// String returns a string representation of the Tag (GGGG,EEEE)
func (t Tag) String() string {
	return fmt.Sprintf("(%04X,%04X)", t.Group, t.Element)
}

// MarshalJSON returns a JSON representation of the Tag
func (t Tag) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}
