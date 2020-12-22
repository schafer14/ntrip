package mock

// TODO: Could probably live in a separate package - maybe in the service package if it were
//  separated from the NTRIP package

import (
	"context"
	"io"
	"time"

	"github.com/go-gnss/ntrip"
)

const (
	MountName string = "TEST00AUS0"
	MountPath string = "/" + MountName
	Username  string = "username"
	Password  string = "password"
)

// MockSourceService implements ntrip.SourceService, copying data from a single connected
// server (TEST00AUS0) into a channel
type MockSourceService struct {
	DataChannel chan []byte
}

func NewMockSourceService() *MockSourceService {
	return &MockSourceService{}
}

func (m *MockSourceService) Sourcetable() string {
	return "CAS;localhost;2101;local;local;0;AUS;-1.0;1.0"
}

func (m *MockSourceService) Subscriber(ctx context.Context, mount string, username string, password string) (chan []byte, error) {
	if username != Username || password != Password {
		return nil, ntrip.ErrorNotAuthorized
	}

	if mount != MountName {
		return nil, ntrip.ErrorNotFound
	}

	if m.DataChannel == nil {
		return nil, ntrip.ErrorNotFound
	}

	return m.DataChannel, nil
}

func (m *MockSourceService) Publisher(ctx context.Context, mount string, username string, password string) (io.WriteCloser, error) {
	if username != Username || password != Password {
		return nil, ntrip.ErrorNotAuthorized
	}

	if mount != MountName {
		return nil, ntrip.ErrorNotFound
	}

	if m.DataChannel != nil {
		return nil, ntrip.ErrorConflict
	}

	m.DataChannel = make(chan []byte, 1)
	return channelWriter(ctx, m), nil
}

// Copies data from the returned WriteCloser to m.DataChannel, closing the channel when WriteCloser is closed
func channelWriter(ctx context.Context, m *MockSourceService) io.WriteCloser {
	r, w := io.Pipe()

	type asyncResp struct { // I wish Go had tuples
		bytesRead int
		err       error
	}

	// Wraps r.Read so it can happen asynchronously, allowing timeouts etc. with select statement
	readAsync := func(buf []byte) chan asyncResp {
		c := make(chan asyncResp, 1)
		go func() {
			br, err := r.Read(buf)
			c <- asyncResp{br, err}
		}()
		return c
	}

	// Read data from r and write to m.DataChannel, with timeouts and context checks
	go func() {
	OUTER:
		for {
			buf := make([]byte, 1024)
			select {
			case resp := <-readAsync(buf):
				if resp.err != nil {
					break OUTER
				}
				m.DataChannel <- buf[:resp.bytesRead]
			case <-time.After(1 * time.Second):
			case <-ctx.Done():
				break OUTER
			}
		}

		// Closing the channel signals to any Subscriber's that the connection should be closed
		close(m.DataChannel)
		// Reset to nil so future calls to Publisher do not return "mount in use" error
		m.DataChannel = nil
	}()

	return w
}
