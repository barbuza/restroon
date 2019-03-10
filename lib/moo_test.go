package roonlib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMoo(t *testing.T) {
	sampleResponse := `MOO/1 CONTINUE Registered
Content-Type: application/json
Request-Id: 11
Content-Length: 202

{"core_id":"a942d85e-acfc-43a7-bdbc-1f12e1029cc2","display_name":"zbox","display_version":"1.6 (build 401) stable","token":"e769f7d4-4ab7-4623-aade-bb159ea84429","provided_services":[],"http_port":9100}`
	requestID, head1, head2, response, err := parseMOOResponse([]byte(sampleResponse))
	assert.Nil(t, err)
	assert.Equal(t, 11, requestID)
	assert.Equal(t, mooContinue, head1)
	assert.Equal(t, "Registered", head2)
	assert.Equal(t, []byte(`{"core_id":"a942d85e-acfc-43a7-bdbc-1f12e1029cc2","display_name":"zbox","display_version":"1.6 (build 401) stable","token":"e769f7d4-4ab7-4623-aade-bb159ea84429","provided_services":[],"http_port":9100}`), response)
}
