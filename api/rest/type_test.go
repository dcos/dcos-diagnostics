package rest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestType_MarshalUnmarshalJSON(t *testing.T) {
	for _, tt := range []struct {
		s      string
		j      string
		actual Type
	}{{"Local", `"Local"`, Local}, {"Cluster", `"Cluster"`, Cluster}} {

		t.Run(tt.s, func(t *testing.T) {
			var actual Type

			err := json.Unmarshal([]byte(tt.j), &actual)
			assert.NoError(t, err)
			assert.Equal(t, tt.actual, actual)

			assert.Equal(t, tt.s, actual.String())

			b, err := json.Marshal(actual)
			assert.NoError(t, err)
			assert.Equal(t, tt.j, string(b))
		})
	}
}

func TestType_UnmarshalJSONWithError(t *testing.T) {
	var actual Type

	err := json.Unmarshal([]byte("L"), &actual)
	assert.EqualError(t, err, "invalid character 'L' looking for beginning of value")

	err = json.Unmarshal([]byte(`"L"`), &actual)
	assert.EqualError(t, err, "\"L\" is not valid type")
}
