package tpm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/go-attestation/attest"

	"go.step.sm/crypto/tpm/manufacturer"
)

type Info struct {
	Version         Version         `json:"version"`
	Interface       Interface       `json:"interface"`
	Manufacturer    Manufacturer    `json:"manufacturer"`
	VendorInfo      string          `json:"vendorInfo,omitempty"`
	FirmwareVersion FirmwareVersion `json:"firmwareVersion,omitempty"`
}

type Version attest.TPMVersion

func (v Version) String() string {
	switch v {
	case Version(attest.TPMVersion12):
		return "TPM 1.2"
	case Version(attest.TPMVersion20):
		return "TPM 2.0"
	default:
		return "unknown"
	}
}

func (v Version) MarshalJSON() ([]byte, error) {
	var s string
	switch v {
	case Version(attest.TPMVersion12):
		s = "1.2"
	case Version(attest.TPMVersion20):
		s = "2.0"
	default:
		s = "unknown"
	}
	return json.Marshal(s)
}

type Interface attest.TPMInterface

func (i Interface) String() string {
	switch i {
	case Interface(attest.TPMInterfaceDirect):
		return "direct"
	case Interface(attest.TPMInterfaceKernelManaged):
		return "kernel-managed"
	case Interface(attest.TPMInterfaceDaemonManaged):
		return "daemon-managed"
	case Interface(attest.TPMInterfaceCommandChannel):
		return "command-channel"
	default:
		return "unknown"
	}
}

func (i Interface) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

type FirmwareVersion struct {
	Major int
	Minor int
}

func (fv FirmwareVersion) String() string {
	return fmt.Sprintf("%d.%d", fv.Major, fv.Minor)
}

func (fv FirmwareVersion) MarshalJSON() ([]byte, error) {
	// TODO(hs): make empty if major.minor is 0.0?
	return json.Marshal(fv.String())
}

// Manufacturer models a TPM Manufacturer.
type Manufacturer struct {
	ID    manufacturer.ID `json:"id"`
	Name  string          `json:"name"`
	ASCII string          `json:"ascii"`
	Hex   string          `json:"hex"`
}

func (m Manufacturer) String() string {
	return fmt.Sprintf("%s (%s, %s, %d)", m.Name, m.ASCII, m.Hex, m.ID)
}

// GetManufacturerByID returns a Manufacturer based on its Manufacturer ID
// code.
func GetManufacturerByID(id manufacturer.ID) Manufacturer {
	ascii, hexa := manufacturer.GetEncodings(id)
	name := manufacturer.GetNameByASCII(ascii)
	return Manufacturer{
		Name:  name,
		ASCII: ascii,
		ID:    id,
		Hex:   hexa,
	}
}

func (t *TPM) Info(ctx context.Context) (*Info, error) {
	if err := t.Open(ctx); err != nil {
		return nil, fmt.Errorf("failed opening TPM: %w", err)
	}
	defer t.Close(ctx)

	info, err := t.attestTPM.Info()
	if err != nil {
		return nil, fmt.Errorf("failed getting TPM info: %w", err)
	}

	result := &Info{
		FirmwareVersion: FirmwareVersion{
			Major: info.FirmwareVersionMajor,
			Minor: info.FirmwareVersionMinor,
		},
		Interface:    Interface(info.Interface),
		Manufacturer: GetManufacturerByID(manufacturer.ID(info.Manufacturer)),
		VendorInfo:   info.VendorInfo,
		Version:      Version(info.Version),
	}

	return result, nil
}
