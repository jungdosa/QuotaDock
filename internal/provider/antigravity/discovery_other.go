//go:build !windows

package antigravity

func discoverLocalEndpoints() ([]endpointCandidate, error) {
	return nil, nil
}
