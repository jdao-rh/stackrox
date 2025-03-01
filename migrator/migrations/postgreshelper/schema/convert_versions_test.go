// Code generated by pg-bindings generator. DO NOT EDIT.
package schema

import (
	"testing"

	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

func TestVersionSerialization(t *testing.T) {
	obj := &storage.Version{}
	assert.NoError(t, testutils.FullInit(obj, testutils.UniqueInitializer(), testutils.JSONFieldsFilter))
	m, err := ConvertVersionFromProto(obj)
	assert.NoError(t, err)
	conv, err := ConvertVersionToProto(m)
	assert.NoError(t, err)
	assert.Equal(t, obj, conv)
}
