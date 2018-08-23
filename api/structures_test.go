package api

import (
	"testing"
	"encoding/json"
	"github.com/stretchr/testify/assert"
)

// this test is created to check if aliased types properly serialize to JSON
func TestUnitSerialization(t *testing.T) {
	t.Parallel()

	given := `{
 "UnitName": "test name",
 "Health": 1,
 "Title": "test title",
 "Timestamp": "0001-01-01T00:00:00Z",
 "PrettyName": "this is very pretty name"
}`

	expected := Unit{
		UnitName: "test name",
		Health: 1,
		Title: "test title",
		PrettyName: "this is very pretty name",
	}
	var actual Unit
	err := json.Unmarshal([]byte(given), &actual)
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	raw, err := json.MarshalIndent(expected, "", " ")
	assert.NoError(t, err)
	assert.Equal(t, given, string(raw))

}
