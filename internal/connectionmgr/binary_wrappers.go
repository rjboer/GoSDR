package connectionmgr

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

func (m *Manager) GetXML(dev uint8) ([]byte, error) {

	log.Printf("GetXML function: %v", dev)
	hdr, plan, err := m.sendBinaryCommand(opPrint, dev, 0, []byte{})
	if err != nil {
		return nil, err
	}
	log.Printf("GetXML function: %v", hdr)

	if hdr == nil || hdr.Opcode != opResponse {
		fmt.Println("unexpected response opcode")
	}

	status, _, xml, err := m.readResponse(plan)
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, fmt.Errorf("GetXML failed: status=%d", status)
	}
	return xml, nil
}

func (m *Manager) Print(dev uint8) ([]byte, error) {

	log.Printf("Print function start device: %v", dev)
	hdr, plan, err := m.sendBinaryCommand(opPrint, dev, 0, []byte{})
	if err != nil {
		return nil, err
	}
	log.Printf("Print function hdr: %v", hdr)

	if hdr == nil || hdr.Opcode != opResponse {
		log.Printf("unexpected response opcode: %v", hdr)
	}

	status, u32, data, err := m.readResponse(plan)
	log.Printf("Print function u32: %v\n", u32)
	log.Printf("Print function status: %v\n", status)
	log.Printf("Print function data: %v\n", data)
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, fmt.Errorf("Print failed: status=%d", status)
	}
	return data, nil
}

func (m *Manager) PrimeCTX(dev uint8) ([]byte, error) {

	log.Printf("PrimeCTX function start device: %v", dev)
	hdr, plan, err := m.sendBinaryCommand(opReadAttr, 1, 0, lpString("sampling_frequency"))
	if err != nil {
		return nil, err
	}
	log.Printf("PrimeCTX function hdr: %v", hdr)

	if hdr == nil || hdr.Opcode != opResponse {
		log.Printf("unexpected response opcode: %v", hdr)
	}

	status, u32, data, err := m.readResponse(plan)
	log.Printf("PrimeCTX function u32: %v\n", u32)
	log.Printf("PrimeCTX function status: %v\n", status)
	log.Printf("PrimeCTX function data: %v\n", data)
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, fmt.Errorf("PrimeCTX failed: status=%d", status)
	}
	return data, nil
}

func (m *Manager) GetSamplingFrequency(dev uint8, idx uint8) (int64, error) {
	hdr, plan, err := m.sendBinaryCommand(opReadAttr, dev, 0, lpString(fmt.Sprintf("sampling_frequency%d", idx)))
	if err != nil {
		return 0, err
	}
	if hdr == nil || hdr.Opcode != opResponse {
		return 0, fmt.Errorf("unexpected response opcode: hdr:%v", hdr)
	}
	status, _, value, err := m.readResponse(plan)
	if err != nil {
		return 0, err
	}
	if status != 0 {
		return 0, fmt.Errorf("GetSamplingFrequency failed: status=%d", status)
	}
	return strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
}

func (m *Manager) GetRFBandwidth(dev uint8, idx uint8) (int64, error) {
	hdr, plan, err := m.sendBinaryCommand(opReadAttr, dev, 0, lpString(fmt.Sprintf("rf_bandwidth%d", idx)))
	if err != nil {
		return 0, err
	}
	if hdr == nil || hdr.Opcode != opResponse {
		return 0, fmt.Errorf("unexpected response opcode")
	}
	status, _, value, err := m.readResponse(plan)
	if err != nil {
		return 0, err
	}
	if status != 0 {
		return 0, fmt.Errorf("GetRFBandwidth failed: status=%d", status)
	}
	return strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
}

func (m *Manager) GetGainControlMode(dev uint8, idx uint8) (string, error) {
	hdr, plan, err := m.sendBinaryCommand(opReadAttr, dev, 0, lpString(fmt.Sprintf("gain_control_mode%d", idx)))
	if err != nil {
		return "", err
	}
	if hdr == nil || hdr.Opcode != opResponse {
		return "", fmt.Errorf("unexpected response opcode")
	}
	status, _, value, err := m.readResponse(plan)
	if err != nil {
		return "", err
	}
	if status != 0 {
		return "", fmt.Errorf("GetGainControlMode failed: status=%d", status)
	}
	return strings.TrimSpace(string(value)), nil
}

func (m *Manager) GetChnAttrIdx(dev uint8, chIdx int32, attr string) (string, error) {
	hdr, plan, err := m.sendBinaryCommand(opReadChnAttr, dev, chIdx, lpString(attr))
	if err != nil {
		return "", err
	}
	if hdr == nil || hdr.Opcode != opResponse {
		return "", fmt.Errorf("GetChnAttrIdx(%d,%s): unexpected response opcode", chIdx, attr)
	}

	status, _, value, err := m.readResponse(plan)
	if err != nil {
		return "", err
	}
	if status != 0 {
		return "", fmt.Errorf("GetChnAttrIdx(%d,%s) failed: status=%d", chIdx, attr, status)
	}
	return strings.TrimSpace(string(value)), nil
}

func (m *Manager) GetBufAttr(dev uint8, name string) (string, error) {
	hdr, plan, err := m.sendBinaryCommand(opReadBufAttr, dev, 0, lpString(name))
	if err != nil {
		return "", err
	}
	if hdr == nil || hdr.Opcode != opResponse {
		return "", fmt.Errorf("GetBufAttr(%s): unexpected response opcode", name)
	}

	status, _, value, err := m.readResponse(plan)
	if err != nil {
		return "", err
	}
	if status != 0 {
		return "", fmt.Errorf("GetBufAttr(%s) failed: status=%d", name, status)
	}
	return strings.TrimSpace(string(value)), nil
}

func (m *Manager) SetBufAttr(dev uint8, name, value string) error {
	hdr, plan, err := m.sendBinaryCommand(opWriteBufAttr, dev, 0, nameValue(name, value))
	if err != nil {
		return err
	}
	if hdr == nil || hdr.Opcode != opResponse {
		return fmt.Errorf("SetBufAttr(%s): unexpected response opcode", name)
	}

	status, _, _, err := m.readResponse(plan)
	if err != nil {
		return err
	}

	if status != 0 {
		return fmt.Errorf("SetBufAttr(%s) failed: status=%d", name, status)
	}
	return nil
}
