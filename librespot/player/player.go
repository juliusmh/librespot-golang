package player

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"sync"

	"github.com/juliusmh/librespot-golang/Spotify"
	"github.com/juliusmh/librespot-golang/librespot/connection"
	"github.com/juliusmh/librespot-golang/librespot/mercury"
)

type Player struct {
	stream   connection.PacketStream
	mercury  *mercury.Client
	seq      uint32
	audioKey []byte

	chanLock    sync.Mutex
	seqChanLock sync.Mutex
	channels    map[uint16]*Channel
	seqChans    sync.Map
	nextChan    uint16
}

func CreatePlayer(conn connection.PacketStream, client *mercury.Client) *Player {
	return &Player{
		stream:   conn,
		mercury:  client,
		channels: map[uint16]*Channel{},
		seqChans: sync.Map{},
		chanLock: sync.Mutex{},
		nextChan: 0,
	}
}

func (p *Player) LoadTrack(file *Spotify.AudioFile, trackId []byte) (*AudioFile, error) {
	return p.LoadTrackWithIdAndFormat(file.FileId, file.GetFormat(), trackId)
}

func (p *Player) LoadTrackWithIdAndFormat(fileId []byte, format Spotify.AudioFile_Format, trackId []byte) (*AudioFile, error) {
	// Allocate an AudioFile and a channel
	audioFile := newAudioFileWithIdAndFormat(fileId, format, p)
	// Start loading the audio key
	err := audioFile.loadKey(trackId)
	// Then start loading the audio itself
	audioFile.loadChunks()
	return audioFile, err
}

func (p *Player) loadTrackKey(trackId []byte, fileId []byte) ([]byte, error) {
	seqInt, seq := p.mercury.NextSeqWithInt()

	p.seqChans.Store(seqInt, make(chan []byte))

	req := buildKeyRequest(seq, trackId, fileId)
	err := p.stream.SendPacket(connection.PacketRequestKey, req)
	if err != nil {
		log.Println("Error while sending packet", err)
		return nil, err
	}

	channel, _ := p.seqChans.Load(seqInt)
	key := <-channel.(chan []byte)
	p.seqChans.Delete(seqInt)

	return key, nil
}

func (p *Player) AllocateChannel() *Channel {
	p.chanLock.Lock()
	channel := NewChannel(p.nextChan, p.releaseChannel)
	p.nextChan++

	p.channels[channel.num] = channel
	p.chanLock.Unlock()

	return channel
}

func (p *Player) HandleCmd(cmd byte, data []byte) error {
	switch {
	case cmd == connection.PacketAesKey:
		// Audio key response
		dataReader := bytes.NewReader(data)
		var seqNum uint32
		err := binary.Read(dataReader, binary.BigEndian, &seqNum)
		if err != nil {
			return fmt.Errorf("could not read binary seqNum: %+v", err)
		}
		if channel, ok := p.seqChans.Load(seqNum); ok {
			channel.(chan []byte) <- data[4:20]
		} else {
			return fmt.Errorf("unknown channel for audio key %d", seqNum)
		}
	case cmd == connection.PacketAesKeyError:
		return fmt.Errorf("audio key error")
	case cmd == connection.PacketStreamChunkRes:
		// Audio data response
		var channel uint16
		dataReader := bytes.NewReader(data)
		err := binary.Read(dataReader, binary.BigEndian, &channel)
		if err != nil {
			return fmt.Errorf("could not read binary channel: %+v", err)
		}
		if val, ok := p.channels[channel]; ok {
			val.handlePacket(data[2:])
		} else {
			return fmt.Errorf("unknown channel")
		}
	}
	return nil
}

func (p *Player) releaseChannel(channel *Channel) {
	p.chanLock.Lock()
	delete(p.channels, channel.num)
	p.chanLock.Unlock()
}
