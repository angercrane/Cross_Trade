/*******************************************************************************
*   (c) 2018 - 2022 ZondaX AG
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

/* THORChain uses same CLA (0x55) as Cosmos app, so okay to use cosmos/ledger-go for transport */
package ledger_thorchain_go

import (
	"errors"
	"fmt"
	"math"

	ledger_go "github.com/cosmos/ledger-go"
)

const (
	userCLA = 0x55

	userINSGetVersion       = 0
	userINSSignSECP256K1    = 2
	userINSGetAddrSecp256k1 = 4

	userMessageChunkSize = 250
)

// LedgerCosmos represents a connection to the Cosmos app in a Ledger Nano S device
type LedgerTHORChain struct {
	api     ledger_go.LedgerDevice
	version VersionInfo
}

// FindLedgerCosmosUserApp finds a Cosmos user app running in a ledger device
func FindLedgerTHORChainUserApp() (_ *LedgerTHORChain, rerr error) {
	ledgerAdmin := ledger_go.NewLedgerAdmin()
	ledgerAPI, err := ledgerAdmin.Connect(0)
	if err != nil {
		return nil, err
	}

	defer func() {
		if rerr != nil {
			ledgerAPI.Close()
		}
	}()

	app := &LedgerTHORChain{ledgerAPI, VersionInfo{}}
	appVersion, err := app.GetVersion()
	if err != nil {
		if err.Error() == "[APDU_CODE_CLA_NOT_SUPPORTED] Class not supported" {
			err = errors.New("are you sure the Cosmos app is open?")
		}
		return nil, err
	}

	if err := app.CheckVersion(*appVersion); err != nil {
		return nil, err
	}

	return app, nil
}

// Close closes a connection with the THORChain user app
func (ledger *LedgerTHORChain) Close() error {
	return ledger.api.Close()
}

// VersionIsSupported returns true if the App version is supported by this library
func (ledger *LedgerTHORChain) CheckVersion(ver VersionInfo) error {
	version, err := ledger.GetVersion()
	if err != nil {
		return err
	}

	switch major := version.Major; major {
	case 1:
		return CheckVersion(ver, VersionInfo{0, 1, 5, 1})
	case 2:
		return CheckVersion(ver, VersionInfo{0, 2, 0, 0})
	default:
		return fmt.Errorf("App version %d is not supported", major)
	}
}

// GetVersion returns the current version of the THORChain user app
func (ledger *LedgerTHORChain) GetVersion() (*VersionInfo, error) {
	message := []byte{userCLA, userINSGetVersion, 0, 0, 0}
	response, err := ledger.api.Exchange(message)

	if err != nil {
		return nil, err
	}

	if len(response) < 4 {
		return nil, errors.New("invalid response")
	}

	ledger.version = VersionInfo{
		AppMode: response[0], //Debug is 0xFF, otherwise 0x00
		Major:   response[1],
		Minor:   response[2],
		Patch:   response[3],
	}

	return &ledger.version, nil
}

// SignSECP256K1 signs a transaction using Cosmos user app. It can either use
// SIGN_MODE_LEGACY_AMINO_JSON (P2=0) or SIGN_MODE_TEXTUAL (P2=1).
// this command requires user confirmation in the device
func (ledger *LedgerTHORChain) SignSECP256K1(bip32Path []uint32, transaction []byte, p2 byte) ([]byte, error) {
	switch major := ledger.version.Major; major {
	case 1:
		return ledger.signv1(bip32Path, transaction)
	case 2:
		return ledger.signv2(bip32Path, transaction, p2)
	default:
		return nil, fmt.Errorf("App version %d is not supported", major)
	}
}

// GetPublicKeySECP256K1 retrieves the public key for the corresponding bip32 derivation path (compressed)
// this command DOES NOT require user confirmation in the device
func (ledger *LedgerTHORChain) GetPublicKeySECP256K1(bip32Path []uint32) ([]byte, error) {
	pubkey, _, err := ledger.getAddressPubKeySECP256K1(bip32Path, "thor", false)
	return pubkey, err
}

func validHRPByte(b byte) bool {
	// https://github.com/bitcoin/bips/blob/master/bip-0173.mediawiki
	return b >= 33 && b <= 126
}

// GetAddressPubKeySECP256K1 returns the pubkey (compressed) and address (bech(
// this command requires user confirmation in the device
func (ledger *LedgerTHORChain) GetAddressPubKeySECP256K1(bip32Path []uint32, hrp string) (pubkey []byte, addr string, err error) {
	return ledger.getAddressPubKeySECP256K1(bip32Path, hrp, true)
}

func (ledger *LedgerTHORChain) GetBip32bytes(bip32Path []uint32, hardenCount int) ([]byte, error) {
	var pathBytes []byte
	var err error

	switch major := ledger.version.Major; major {
	case 1:
		pathBytes, err = GetBip32bytesv1(bip32Path, 3)
		if err != nil {
			return nil, err
		}
	case 2:
		pathBytes, err = GetBip32bytesv2(bip32Path, 3)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("App version %d is not supported", major)
	}

	return pathBytes, nil
}

func (ledger *LedgerCosmos) signv1(bip32Path []uint32, transaction []byte) ([]byte, error) {
	var packetIndex byte = 1
	var packetCount = 1 + byte(math.Ceil(float64(len(transaction))/float64(userMessageChunkSize)))

	var finalResponse []byte

	var message []byte

	for packetIndex <= packetCount {
		chunk := userMessageChunkSize
		if packetIndex == 1 {
			pathBytes, err := ledger.GetBip32bytes(bip32Path, 3)
			if err != nil {
				return nil, err
			}
			header := []byte{userCLA, userINSSignSECP256K1, packetIndex, packetCount, byte(len(pathBytes))}
			message = append(header, pathBytes...)
		} else {
			if len(transaction) < userMessageChunkSize {
				chunk = len(transaction)
			}
			header := []byte{userCLA, userINSSignSECP256K1, packetIndex, packetCount, byte(chunk)}
			message = append(header, transaction[:chunk]...)
		}

		response, err := ledger.api.Exchange(message)
		if err != nil {
			if err.Error() == "[APDU_CODE_BAD_KEY_HANDLE] The parameters in the data field are incorrect" {
				// In this special case, we can extract additional info
				errorMsg := string(response)
				switch errorMsg {
				case "ERROR: JSMN_ERROR_NOMEM":
					return nil, errors.New("Not enough tokens were provided")
				case "PARSER ERROR: JSMN_ERROR_INVAL":
					return nil, errors.New("Unexpected character in JSON string")
				case "PARSER ERROR: JSMN_ERROR_PART":
					return nil, errors.New("The JSON string is not a complete.")
				}
				return nil, errors.New(errorMsg)
			}
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

func (ledger *LedgerCosmos) signv2(bip32Path []uint32, transaction []byte, p2 byte) ([]byte, error) {
	var packetIndex byte = 1
	var packetCount = 1 + byte(math.Ceil(float64(len(transaction))/float64(userMessageChunkSize)))

	var finalResponse []byte

	var message []byte

	if p2 > 1 {
		return nil, errors.New("only values of SIGN_MODE_LEGACY_AMINO (P2=0) and SIGN_MODE_TEXTUAL (P2=1) are allowed")
	}

	for packetIndex <= packetCount {
		chunk := userMessageChunkSize
		if packetIndex == 1 {
			pathBytes, err := ledger.GetBip32bytes(bip32Path, 3)
			if err != nil {
				return nil, err
			}
			header := []byte{userCLA, userINSSignSECP256K1, 0, p2, byte(len(pathBytes))}
			message = append(header, pathBytes...)
		} else {
			if len(transaction) < userMessageChunkSize {
				chunk = len(transaction)
			}

			payloadDesc := byte(1)
			if packetIndex == packetCount {
				payloadDesc = byte(2)
			}

			header := []byte{userCLA, userINSSignSECP256K1, payloadDesc, p2, byte(chunk)}
			message = append(header, transaction[:chunk]...)
		}

		response, err := ledger.api.Exchange(message)
		if err != nil {
			if err.Error() == "[APDU_CODE_BAD_KEY_HANDLE] The parameters in the data field are incorrect" {
				// In this special case, we can extract additional info
				errorMsg := string(response)
				switch errorMsg {
				case "ERROR: JSMN_ERROR_NOMEM":
					return nil, errors.New("Not enough tokens were provided")
				case "PARSER ERROR: JSMN_ERROR_INVAL":
					return nil, errors.New("Unexpected character in JSON string")
				case "PARSER ERROR: JSMN_ERROR_PART":
					return nil, errors.New("The JSON string is not a complete.")
				}
				return nil, errors.New(errorMsg)
			}
			if err.Error() == "[APDU_CODE_DATA_INVALID] Referenced data reversibly blocked (invalidated)" {
				errorMsg := string(response)
				return nil, errors.New(errorMsg)
			}
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

// GetAddressPubKeySECP256K1 returns the pubkey (compressed) and address (bech(
// this command requires user confirmation in the device
func (ledger *LedgerTHORChain) getAddressPubKeySECP256K1(bip32Path []uint32, hrp string, requireConfirmation bool) (pubkey []byte, addr string, err error) {
	if len(hrp) > 83 {
		return nil, "", errors.New("hrp len should be <10")
	}

	hrpBytes := []byte(hrp)
	for _, b := range hrpBytes {
		if !validHRPByte(b) {
			return nil, "", errors.New("all characters in the HRP must be in the [33, 126] range")
		}
	}

	pathBytes, err := ledger.GetBip32bytes(bip32Path, 3)
	if err != nil {
		return nil, "", err
	}

	p1 := byte(0)
	if requireConfirmation {
		p1 = byte(1)
	}

	// Prepare message
	header := []byte{userCLA, userINSGetAddrSecp256k1, p1, 0, 0}
	message := append(header, byte(len(hrpBytes)))
	message = append(message, hrpBytes...)
	message = append(message, pathBytes...)
	message[4] = byte(len(message) - len(header)) // update length

	response, err := ledger.api.Exchange(message)

	if err != nil {
		return nil, "", err
	}
	if len(response) < 35+len(hrp) {
		return nil, "", errors.New("Invalid response")
	}

	pubkey = response[0:33]
	addr = string(response[33:])

	return pubkey, addr, err
}
