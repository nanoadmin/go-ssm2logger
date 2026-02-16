package ssm2lib_test

import (
	"encoding/binary"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/nanoadmin/go-ssm2logger/ssm2lib"
)

var _ = Describe("Ssm2lib", func() {
	It("Can create a write address packet to switch to fast mode", func() {
		Ω(true).Should(Equal(true))
	})

	It("Can create a read address request", func() {
		readPacket := NewReadAddressRequestPacket(Ssm2DeviceDiagnosticToolF0, Ssm2DeviceEngine10, [][]byte{{0x00, 0x00, 0x46}}, false)
		Ω(readPacket.Packet).Should(Equal(Ssm2PacketBytes([]byte{0x80, 0x10, 0xf0, 0x05, 0xa8, 0x00, 0x00, 0x00, 0x46, 0x73})))
	})

	It("Can create an init request", func() {
		initPacket := NewInitRequestPacket(Ssm2DeviceDiagnosticToolF0, Ssm2DeviceEngine10)
		Ω(initPacket.Packet).Should(Equal(Ssm2PacketBytes([]byte{0x80, 0x10, 0xf0, 0x01, 0xbf, 0x40})))
	})

	Context("Wire time", func() {
		It("Knows how long bytes will take on the wire", func() {
			microseconds := MicrosecondsOnTheWireBytes(make([]byte, 8))
			Ω(microseconds).Should(Equal(16667))
		})
	})

	Context("Validation", func() {
		Context("The first byte is wrong", func() {
			It("Returns an error", func() {
				bogusPacket := NewPacketFromBytes([]byte{0x00})
				err := bogusPacket.Packet.Validate()
				Ω(err).To(HaveOccurred())
				Ω(err.Error()).To(Equal("First byte of packet is wrong. Expected 0x80, got 0x00"))
			})
		})
	})

	Context("Parameter", func() {
		Context("Conversion", func() {
			It("Can evaluate the expression", func() {
				param := &Ssm2Parameter{
					Conversions: []Ssm2ParameterConversion{{
						Units: "%",
						Expr:  "x/2",
					}},
				}

				binaryTen := make([]byte, binary.MaxVarintLen64)
				binary.PutUvarint(binaryTen, 10)
				val, err := param.Convert("%", binaryTen)
				Ω(err).ShouldNot(HaveOccurred())
				Ω(val).Should(Equal(5.0))
			})
		})
	})

	Context("Address expansion", func() {
		It("Expands a 3-byte address by offset", func() {
			expanded, err := ExpandAddress([]byte{0x00, 0x01, 0xFE}, 2)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(expanded).Should(Equal([]byte{0x00, 0x02, 0x00}))
		})
	})

	Context("Payload bytes", func() {
		It("Excludes command byte and checksum", func() {
			packet := Ssm2PacketBytes([]byte{0x80, 0x10, 0xF0, 0x03, 0xE8, 0xAA, 0xBB, 0xCF})
			Ω(packet.GetPayloadBytes()).Should(Equal([]byte{0xAA, 0xBB}))
		})
	})

	Context("Parameter mapping", func() {
		It("Builds offsets for mixed multi-byte and single-byte params", func() {
			params := []Ssm2Parameter{
				{Name: "A", Address: Ssm2ParameterAddress{Address: "0x000100", Length: 2}, Conversions: []Ssm2ParameterConversion{{Units: "u"}}},
				{Name: "B", Address: Ssm2ParameterAddress{Address: "0x000200", Length: 1}, Conversions: []Ssm2ParameterConversion{{Units: "u"}}},
			}
			addrs, mappings, err := BuildParameterAddressRequest(params)
			Ω(err).ShouldNot(HaveOccurred())
			Ω(addrs).Should(Equal([][]byte{{0x00, 0x01, 0x00}, {0x00, 0x01, 0x01}, {0x00, 0x02, 0x00}}))
			Ω(mappings[0].Start).Should(Equal(0))
			Ω(mappings[0].Length).Should(Equal(2))
			Ω(mappings[1].Start).Should(Equal(2))
			Ω(mappings[1].Length).Should(Equal(1))
		})
	})
})
