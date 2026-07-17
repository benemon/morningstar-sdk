//go:build integration

package mc8pro

// SendRawForTest exposes the raw port send for integration-test
// protocol probes. Test-only: this file is excluded from normal
// builds by the integration tag.
func (c *Client) SendRawForTest(frame []byte) error {
	return c.port.Send(frame)
}
