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

package ledger_cosmos_go

import (
	"github.com/ZondaX/ledger-go"
	"fmt"
	"math"
)

const (
	userCLA = 0x55

	userINSGetVersion         = 0
	userINSPublicKeySECP256K1 = 1
	userINSPublicKeyED25519   = 2

	// Sign sdk.Msg (transaction part of the message)
	userINSSignSECP256K1 = 3
	userINSSignED25519   = 4

	// Sign sdk.StdSignMsg (full message)
	userINSSignSECP256K1_StdSignMsg = 5
	userINSSignED25519_StdSignMsg   = 6

	userINSHash                   = 100
	userINSPublicKeySECP256K1Test = 101
	userINSPublicKeyED25519Test   = 102
	userINSSignSECP256K1Test      = 103
	userINSSignED25519Test        = 104

	userMessageChunkSize = 250
)

// User app
type LedgerCosmos struct {
	api *ledger_go.Ledger
}

func FindLedgerCosmos() (*LedgerCosmos, error) {
	ledgerApi, err := ledger_go.FindLedger()
	// TODO: Check version number here
	return &LedgerCosmos{ledgerApi}, err
}

func (ledger *LedgerCosmos) GetVersion() (*VersionInfo, error) {
	message := []byte{userCLA, userINSGetVersion, 0, 0, 0}
	response, err := ledger.api.Exchange(message)

	if err != nil {
		return nil, err
	}

	if len(response) < 4 {
		return nil, fmt.Errorf("invalid response")
	}

	return &VersionInfo{
		AppId: response[0],
		Major: response[1],
		Minor: response[2],
		Patch: response[3],
	}, nil
}

func (ledger *LedgerCosmos) sign(instruction byte, bip32_path []uint32, transaction []byte) ([]byte, error) {
	var packetIndex byte = 1
	var packetCount byte = 1 + byte(math.Ceil(float64(len(transaction))/float64(userMessageChunkSize)))

	var finalResponse []byte

	var message []byte

	for packetIndex <= packetCount {
		chunk := userMessageChunkSize
		if packetIndex == 1 {
			pathBytes, err := getBip32bytes(bip32_path, 3)
			if err != nil {
				return nil, err
			}
			header := []byte{userCLA, instruction, packetIndex, packetCount, byte(len(pathBytes))}
			message = append(header, pathBytes...)
		} else {
			if len(transaction) < userMessageChunkSize {
				chunk = len(transaction)
			}
			header := []byte{userCLA, instruction, packetIndex, packetCount, byte(chunk)}
			message = append(header, transaction[:chunk]...)
		}

		response, err := ledger.api.Exchange(message)
		if err != nil {
			return nil, err
		}

		finalResponse = response
		if packetIndex > 1 {
			transaction = transaction[chunk:]
		}
		packetIndex++

	}
	return finalResponse, nil
}

func (ledger *LedgerCosmos) SignSECP256K1(bip32_path []uint32, transaction []byte) ([]byte, error) {
	return ledger.sign(userINSSignSECP256K1, bip32_path, transaction)
}

func (ledger *LedgerCosmos) SignED25519(bip32_path []uint32, transaction []byte) ([]byte, error) {
	return ledger.sign(userINSSignED25519, bip32_path, transaction)
}

func (ledger *LedgerCosmos) SignSECP256K1_StdSignMsg(bip32_path []uint32, transaction []byte) ([]byte, error) {
	return ledger.sign(userINSSignSECP256K1_StdSignMsg, bip32_path, transaction)
}

func (ledger *LedgerCosmos) SignED25519_StdSignMsg(bip32_path []uint32, transaction []byte) ([]byte, error) {
	return ledger.sign(userINSSignED25519_StdSignMsg, bip32_path, transaction)
}

func (ledger *LedgerCosmos) GetPublicKeySECP256K1(bip32_path []uint32) ([]byte, error) {
	pathBytes, err := getBip32bytes(bip32_path, 3)
	if err != nil {
		return nil, err
	}
	header := []byte{userCLA, userINSPublicKeySECP256K1, 0, 0, byte(len(pathBytes))}
	message := append(header, pathBytes...)

	response, err := ledger.api.Exchange(message)

	if err != nil {
		return nil, err
	}

	if len(response) < 4 {
		return nil, fmt.Errorf("invalid response")
	}

	return response, nil
}

func (ledger *LedgerCosmos) GetPublicKeyED25519(bip32_path []uint32) ([]byte, error) {
	pathBytes, err := getBip32bytes(bip32_path, 3)
	if err != nil {
		return nil, err
	}

	header := []byte{userCLA, userINSPublicKeyED25519, 0, 0, byte(len(pathBytes))}
	message := append(header, pathBytes...)

	response, err := ledger.api.Exchange(message)

	if err != nil {
		return nil, err
	}

	if len(response) < 4 {
		return nil, fmt.Errorf("invalid response")
	}

	return response, nil
}

func (ledger *LedgerCosmos) Hash(transaction []byte) ([]byte, error) {

	var packetIndex = byte(1)
	var packetCount = byte(math.Ceil(float64(len(transaction)) / float64(userMessageChunkSize)))

	var finalResponse []byte
	for packetIndex <= packetCount {
		chunk := userMessageChunkSize
		if len(transaction) < userMessageChunkSize {
			chunk = len(transaction)
		}

		header := []byte{userCLA, userINSHash, packetIndex, packetCount, byte(chunk)}
		message := append(header, transaction[:chunk]...)
		response, err := ledger.api.Exchange(message)

		if err != nil {
			return nil, err
		}
		finalResponse = response
		packetIndex++
		transaction = transaction[chunk:]
	}
	return finalResponse, nil
}

func (ledger *LedgerCosmos) TestGetPublicKeySECP256K1() ([]byte, error) {
	message := []byte{userCLA, userINSPublicKeySECP256K1Test, 0, 0, 0}
	response, err := ledger.api.Exchange(message)

	if err != nil {
		return nil, err
	}

	if len(response) < 4 {
		return nil, fmt.Errorf("invalid response")
	}

	return response, nil
}

func (ledger *LedgerCosmos) TestGetPublicKeyED25519() ([]byte, error) {
	message := []byte{userCLA, userINSPublicKeyED25519Test, 0, 0, 0}
	response, err := ledger.api.Exchange(message)

	if err != nil {
		return nil, err
	}

	if len(response) < 4 {
		return nil, fmt.Errorf("invalid response")
	}

	return response, nil
}

func (ledger *LedgerCosmos) TestSignSECP256K1(transaction []byte) ([]byte, error) {
	var packetIndex byte = 1
	var packetCount byte = byte(math.Ceil(float64(len(transaction)) / float64(userMessageChunkSize)))

	var finalResponse []byte

	for packetIndex <= packetCount {

		chunk := userMessageChunkSize
		if len(transaction) < userMessageChunkSize {
			chunk = len(transaction)
		}

		header := []byte{userCLA, userINSSignSECP256K1Test, packetIndex, packetCount, byte(chunk)}
		message := append(header, transaction[:chunk]...)

		response, err := ledger.api.Exchange(message)

		if err != nil {
			return nil, err
		}

		finalResponse = response
		packetIndex++
		transaction = transaction[chunk:]
	}
	return finalResponse, nil
}

func (ledger *LedgerCosmos) TestSignED25519(transaction []byte) ([]byte, error) {
	var packetIndex byte = 1
	var packetCount byte = byte(math.Ceil(float64(len(transaction)) / float64(userMessageChunkSize)))

	var finalResponse []byte

	for packetIndex <= packetCount {
		chunk := userMessageChunkSize
		if len(transaction) < userMessageChunkSize {
			chunk = len(transaction)
		}
		header := []byte{userCLA, userINSSignED25519Test, packetIndex, packetCount, byte(chunk)}
		message := append(header, transaction[:chunk]...)

		response, err := ledger.api.Exchange(message)

		if err != nil {
			return nil, err
		}

		finalResponse = response
		packetIndex++
		transaction = transaction[chunk:]
	}
	return finalResponse, nil
}
