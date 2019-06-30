package player

import (
	"bytes"
	"encoding/binary"
)

type headerFunc func(channel *Channel, id byte, data *bytes.Reader) uint16
type dataFunc func(channel *Channel, data []byte) uint16
type releaseFunc func(channel *Channel)

type Channel struct {
	num       uint16
	dataMode  bool
	onHeader  headerFunc
	onData    dataFunc
	onRelease releaseFunc
}

func NewChannel(num uint16, release releaseFunc) *Channel {
	return &Channel{
		num:       num,
		dataMode:  false,
		onRelease: release,
	}
}

func (c *Channel) handlePacket(data []byte) {
	dataReader := bytes.NewReader(data)

	if !c.dataMode {
		// Read the header
		length := uint16(0)
		var err error = nil
		for err == nil {
			err = binary.Read(dataReader, binary.BigEndian, &length)

			if err != nil {
				break
			}

			if length > 0 {
				var headerId uint8
				binary.Read(dataReader, binary.BigEndian, &headerId)

				read := uint16(0)
				if c.onHeader != nil {
					read = c.onHeader(c, headerId, dataReader)
				}

				// Consume the remaining un-read data
				dataReader.Read(make([]byte, length-read))
			}
		}

		if c.onData != nil {
			c.dataMode = true
		} else {
			c.onRelease(c)
		}
	} else {
		if len(data) == 0 {
			if c.onData != nil {
				c.onData(c, nil)
			}

			c.onRelease(c)
		} else {
			if c.onData != nil {
				c.onData(c, data)
			}
		}
	}

}
