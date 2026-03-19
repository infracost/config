package autodetect

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_envFromLayerName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "production", "production"},
		{"with subenv", "production@us-east-1", "production"},
		{"with instance", "production_foo", "production"},
		{"with subenv and instance", "production@us-east-1_foo", "production"},
		{"instance with underscore", "production_foo_bar", "production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, envFromLayerName(tt.input))
		})
	}
}
