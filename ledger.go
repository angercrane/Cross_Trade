/*******************************************************************************
*   (c) 2018 ZondaX GmbH
*
*  Licensed under the Apache License, Version 2.0 (the "License");
*  you may not use this file except in compliance with the License.
*  You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
*  Unless required by applicable law or agreed to in writing, software
*  distributed under the License is distributed on an "AS IS" BASIS,
*  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
*  See the License for the specific language governing permissions and
*  limitations under the License.
********************************************************************************/

package ledger_goclient

import (
	"fmt"
	"errors"
	"github.com/brejski/hid"
	"math"
)

const (
	VendorLedger     = 0x2c97
	ProductNano      = 1
	Channel          = 0x8001 // TODO: Check. This originally was 0x0101
	PacketSize       = 64
	CLA              = 0x80
	SignInstruction  = 0x01
	MessageChunkSize = 250
)

type Ledger struct {
	device Device
}

func NewLedger(dev Device) *Ledger {
	return &Ledger{
		device: dev,
	}
}

func FindLedger() (*Ledger, error) {
	devices, err := hid.Devices()
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		if d.VendorID == VendorLedger {
			ledger, err := d.Open()
			if err != nil {
				return nil, err
			}
			return NewLedger(ledger), nil
		}
	}
	return nil, errors.New("no ledger connected")
}

// A Device provides access to a HID device.
type Device interface {
	// Close closes the device and associated resources.
	Close()

	// Write writes an output report to device. The first byte must be the
	// report number to write, zero if the device does not use numbered reports.
	Write([]byte) error

	// ReadCh returns a channel that will be sent input reports from the device.
	// If the device uses numbered reports, the first byte will be the report
	// number.
	ReadCh() <-chan []byte

	// ReadError returns the read error, if any after the channel returned from
	// ReadCh has been closed.
	ReadError() error
}

func (ledger *Ledger) Exchange(command []byte) ([]byte, error) {
	serializedCommand, err := WrapCommandAPDU(Channel, command, PacketSize, false)

	if err != nil {
		return nil, err
	}

	// Write all the packets
	err = ledger.device.Write(serializedCommand[:PacketSize])
	if err != nil {
		return nil, err
	}
	for len(serializedCommand) > PacketSize {
		serializedCommand = serializedCommand[PacketSize:]
		err = ledger.device.Write(serializedCommand[:PacketSize])
		if err != nil {
			return nil, err
		}
	}

	input := ledger.device.ReadCh()
	response, err := UnwrapResponseAPDU(Channel, input, PacketSize, false)

	if len(response) < 2 {
		return nil, fmt.Errorf("lost connection")
	}

	swOffset := len(response) - 2
	sw := codec.Uint16(response[swOffset:])
	if sw != 0x9000 {
		// TODO: parse APDU error codes
		return nil, fmt.Errorf("invalid status %04x", sw)
	}
	
	return response[:swOffset], nil
}

func (ledger *Ledger) Sign(transaction []byte) ([]byte, error) {

	var packetIndex byte = 1
	var packetCount byte = byte(math.Ceil(float64(len(transaction)) / float64(MessageChunkSize)))

	var finalResponse []byte
	for packetIndex <= packetCount {
		header := make([]byte, 4)
		header[0] = CLA;
		header[1] = SignInstruction;
		header[2] = packetIndex
		header[3] = packetCount

		chunk := MessageChunkSize
		if len(transaction) < MessageChunkSize {
			chunk = len(transaction)
		}
		message := append(header, transaction[:chunk]...)
		response, err := ledger.Exchange(message)

		if err != nil {
			return nil, err
		}
		finalResponse = response
		packetIndex++
		transaction = transaction[chunk:]
	}
	return finalResponse, nil
}
