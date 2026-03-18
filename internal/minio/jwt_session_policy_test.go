package minio

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realSAToken is the sessionToken from the sample service account identity.json
// provided during development. It contains an embedded-policy with admin:* + s3:*.
const realSAToken = "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJhY2Nlc3NLZXkiOiJBSkhDRTMwbXprTkpvODJwIiwicGFyZW50IjoiR09ETU9ERSIsInNhLXBvbGljeSI6ImVtYmVkZGVkLXBvbGljeSIsInNlc3Npb25Qb2xpY3kiOiJleUpXWlhKemFXOXVJam9pTWpBeE1pMHhNQzB4TnlJc0lsTjBZWFJsYldWdWRDSTZXM3NpUldabVpXTjBJam9pUVd4c2IzY2lMQ0pCWTNScGIyNGlPbHNpWVdSdGFXNDZLaUpkZlN4N0lrVm1abVZqZENJNklrRnNiRzkzSWl3aVFXTjBhVzl1SWpwYkltdHRjem9xSWwxOUxIc2lSV1ptWldOMElqb2lRV3hzYjNjaUxDSkJZM1JwYjI0aU9sc2ljek02S2lKZExDSlNaWE52ZFhKalpTSTZXeUpoY200NllYZHpPbk16T2pvNktpSmRmVjE5In0.L-SNwVJr2DsiJI4pTRfOINCpy4dV03L5TdoIazPjVut9aF1qH5I21o3FmyYzudV4phWD8qDSP4IxmFpstmGlQg"

func TestExtractJWTSessionPolicy_EmbeddedPolicy(t *testing.T) {
	policyJSON, err := extractJWTSessionPolicy(realSAToken)
	require.NoError(t, err)
	require.NotEmpty(t, policyJSON, "should have extracted a session policy")

	// Verify it is valid JSON with expected structure
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(policyJSON), &doc))
	assert.Equal(t, "2012-10-17", doc["Version"])
	stmts, ok := doc["Statement"].([]any)
	require.True(t, ok, "Statement should be an array")
	assert.NotEmpty(t, stmts)
}

func TestExtractJWTSessionPolicy_NoPolicy(t *testing.T) {
	// A JWT whose payload has sa-policy != "embedded-policy"
	// Header: {"alg":"HS512","typ":"JWT"} → eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9
	// Payload: {"accessKey":"test","parent":"user","sa-policy":"inherited-policy"}
	token := "eyJhbGciOiJIUzUxMiIsInR5cCI6IkpXVCJ9.eyJhY2Nlc3NLZXkiOiJ0ZXN0IiwicGFyZW50IjoidXNlciIsInNhLXBvbGljeSI6ImluaGVyaXRlZC1wb2xpY3kifQ.fakesig"
	policyJSON, err := extractJWTSessionPolicy(token)
	require.NoError(t, err)
	assert.Empty(t, policyJSON, "inherited-policy SA should return empty string")
}

func TestExtractJWTSessionPolicy_InvalidJWT(t *testing.T) {
	_, err := extractJWTSessionPolicy("not.a.valid.jwt.with.too.many.parts")
	assert.Error(t, err)

	_, err = extractJWTSessionPolicy("onlytwoparts")
	assert.Error(t, err)
}
