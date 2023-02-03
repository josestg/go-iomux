package iomux

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"testing"
)

var networks = []string{
	"unix", "unixgram", "unixpacket",
}

func TestMuxReadWhileErr(t *testing.T) {
	mux, err := NewMux[string]()
	assert.Nil(t, err)
	_, err = mux.Tag("a")
	assert.Nil(t, err)

	expected := errors.New("this is an error")
	_, err = mux.ReadWhile(func() error {
		return expected
	})

	assert.ErrorIs(t, expected, err)
}

func TestMuxReadNoSenders(t *testing.T) {
	mux, err := NewMux[string]()
	assert.Nil(t, err)

	data, tag, err := mux.Read()

	assert.Nil(t, data)
	assert.Equal(t, "", tag)
	assert.ErrorIs(t, err, MuxNoConnections)
}

func TestMuxReadClosed(t *testing.T) {
	mux, _ := NewMux[string]()
	mux.Close()
	_, _, err := mux.Read()

	assert.ErrorIs(t, err, MuxClosed)
}

func TestMux(t *testing.T) {
	for _, network := range networks {
		t.Run(network, func(t *testing.T) {
			mux, err := newMux[string](network)
			if err != nil {
				skipIfProtocolNotSupported(t, err, network)
			}
			assert.Nil(t, err)
			taga, _ := mux.Tag("a")
			tagb, _ := mux.Tag("b")
			assert.Nil(t, err)

			td, err := mux.ReadWhile(func() error {
				io.WriteString(taga, "hello taga")
				io.WriteString(tagb, "hello tagb")
				return nil
			})

			assert.Equal(t, 2, len(td))
			assert.Equal(t, "hello taga", string(td[0].Data))
			assert.Equal(t, "hello tagb", string(td[1].Data))
		})
	}
}

func skipIfProtocolNotSupported(t *testing.T, err error, network string) {
	err = errors.Unwrap(err)
	if sys, ok := err.(*os.SyscallError); ok {
		if sys.Syscall == "socket" {
			err = errors.Unwrap(err)
			if err == unix.EPROTONOSUPPORT {
				t.Skip("unsupported protocol")
			}
		}
	}
}

func TestMuxMultiple(t *testing.T) {
	for _, network := range networks {
		t.Run(network, func(t *testing.T) {
			mux, err := newMux[string](network)
			if err != nil {
				skipIfProtocolNotSupported(t, err, network)
			}
			taga, _ := mux.Tag("a")
			tagb, _ := mux.Tag("b")
			tagc, _ := mux.Tag("c")
			assert.Nil(t, err)

			td, err := mux.ReadWhile(func() error {
				io.WriteString(taga, "out1")
				io.WriteString(tagb, "err1")
				io.WriteString(tagb, "err2")
				io.WriteString(tagc, "other")
				return nil
			})

			if len(td) == 3 && network != "unixgram" {
				// not message based
				assert.Equal(t, 3, len(td))
				out1 := td[0]
				assert.Equal(t, "a", out1.Tag)
				assert.Equal(t, "out1", string(out1.Data))
				err1 := td[1]
				assert.Equal(t, "b", err1.Tag)
				assert.Equal(t, "err1err2", string(err1.Data))
				out2 := td[2]
				assert.Equal(t, "c", out2.Tag)
				assert.Equal(t, "other", string(out2.Data))
			} else {
				assert.Equal(t, 4, len(td))
				out1 := td[0]
				assert.Equal(t, "a", out1.Tag)
				assert.Equal(t, "out1", string(out1.Data))
				err1 := td[1]
				assert.Equal(t, "b", err1.Tag)
				assert.Equal(t, "err1", string(err1.Data))
				err2 := td[2]
				assert.Equal(t, "b", err2.Tag)
				assert.Equal(t, "err2", string(err2.Data))
				out2 := td[3]
				assert.Equal(t, "c", out2.Tag)
				assert.Equal(t, "other", string(out2.Data))
			}
		})
	}
}

func TestMuxCmd(t *testing.T) {
	for _, network := range networks {
		t.Run(network, func(t *testing.T) {
			mux, err := newMux[int](network)
			if err != nil {
				skipIfProtocolNotSupported(t, err, network)
			}
			cmd := exec.Command("sh", "-c", "echo out1 && echo err1 1>&2 && echo out2")
			stdout, _ := mux.Tag(0)
			stderr, _ := mux.Tag(1)
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			cmd.Run()
			td, err := mux.ReadWhile(func() error {
				err := cmd.Run()
				return err
			})

			if len(td) == 2 && network != "unixgram" {
				// not message based
				out1 := td[0]
				assert.Equal(t, 0, out1.Tag)
				assert.Equal(t, "out1\nout2\n", string(out1.Data))
			} else {
				assert.Equal(t, 3, len(td))
				out1 := td[0]
				assert.Equal(t, 0, out1.Tag)
				assert.Equal(t, "out1\n", string(out1.Data))
				out2 := td[2]
				assert.Equal(t, 0, out2.Tag)
				assert.Equal(t, "out2\n", string(out2.Data))
			}
			err1 := td[1]
			assert.Equal(t, 1, err1.Tag)
			assert.Equal(t, "err1\n", string(err1.Data))
		})
	}
}
