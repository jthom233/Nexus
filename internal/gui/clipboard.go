package gui

import (
	"bytes"
	"log"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/tomatome/grdp/core"
	"github.com/tomatome/grdp/plugin"
)

// CLIPRDR message types (defined locally to avoid importing the Windows-only
// plugin/cliprdr package).
const (
	cbMonitorReady       = 0x0001
	cbFormatList         = 0x0002
	cbFormatListResponse = 0x0003
	cbFormatDataRequest  = 0x0004
	cbFormatDataResponse = 0x0005
	cbClipCaps           = 0x0007
)

// Message flags.
const (
	cbResponseOK   = 0x0001
	cbResponseFail = 0x0002
)

// Clipboard format.
const cfUnicodeText = 13

// Capability flags.
const (
	cbCapsTypeGeneral    = 1
	cbCapsVersion2       = 2
	cbUseLongFormatNames = 0x00000002
)

// isTextContent reports whether s looks like plain text rather than binary
// data. It checks well-known image magic bytes and falls back to a density
// heuristic: if more than 12.5% of the first 512 bytes are non-printable
// (excluding common whitespace), the content is treated as binary.
func isTextContent(s string) bool {
	if len(s) < 4 {
		return true
	}
	b := []byte(s)
	// PNG
	if b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G' {
		return false
	}
	// JPEG
	if b[0] == 0xFF && b[1] == 0xD8 {
		return false
	}
	// GIF
	if b[0] == 'G' && b[1] == 'I' && b[2] == 'F' && b[3] == '8' {
		return false
	}
	// BMP
	if b[0] == 'B' && b[1] == 'M' {
		return false
	}
	// WebP (RIFF....WEBP)
	if b[0] == 'R' && b[1] == 'I' && b[2] == 'F' && b[3] == 'F' {
		return false
	}
	// Generic binary detection: high density of non-printable bytes in first 512.
	check := 512
	if check > len(b) {
		check = len(b)
	}
	nonPrint := 0
	for _, c := range b[:check] {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
			nonPrint++
		}
	}
	return nonPrint < check/8
}

// clipboardChannel implements plugin.ChannelTransport for the CLIPRDR virtual
// channel. It synchronises the local system clipboard with the remote RDP
// session using CF_UNICODETEXT.
type clipboardChannel struct {
	sender      core.ChannelSender
	mu          sync.Mutex
	lastContent string
	stopPoll    chan struct{}
	ready       bool
}

// newClipboardChannel returns an initialised clipboardChannel.
func newClipboardChannel() *clipboardChannel {
	return &clipboardChannel{
		stopPoll: make(chan struct{}),
	}
}

// GetType returns the channel name and option flags required by the RDP
// virtual channel infrastructure.
func (c *clipboardChannel) GetType() (string, uint32) {
	return plugin.CLIPRDR_SVC_CHANNEL_NAME,
		plugin.CHANNEL_OPTION_INITIALIZED | plugin.CHANNEL_OPTION_ENCRYPT_RDP |
			plugin.CHANNEL_OPTION_COMPRESS_RDP | plugin.CHANNEL_OPTION_SHOW_PROTOCOL
}

// Sender stores the channel sender used to transmit PDUs back to the server.
func (c *clipboardChannel) Sender(s core.ChannelSender) {
	c.sender = s
}

// Process handles incoming CLIPRDR PDUs from the server.
func (c *clipboardChannel) Process(s []byte) {
	if len(s) < 8 {
		return
	}

	r := bytes.NewReader(s[:8])
	msgType, _ := core.ReadUint16LE(r)
	msgFlags, _ := core.ReadUint16LE(r)
	dataLen, _ := core.ReadUInt32LE(r)

	var payload []byte
	if dataLen > 0 && len(s) >= 8+int(dataLen) {
		payload = s[8 : 8+dataLen]
	}

	switch msgType {
	case cbClipCaps:
		log.Println("cliprdr: server capabilities received")
	case cbMonitorReady:
		log.Println("cliprdr: server monitor ready")
		c.mu.Lock()
		c.ready = true
		c.mu.Unlock()
		c.sendCapabilities()
		c.sendFormatList()
		go c.pollClipboard()
	case cbFormatList:
		c.processFormatList(payload)
	case cbFormatListResponse:
		// Acknowledgement from server, nothing to do.
	case cbFormatDataRequest:
		c.processFormatDataRequest(payload)
	case cbFormatDataResponse:
		c.processFormatDataResponse(msgFlags, payload)
	default:
		log.Printf("cliprdr: unhandled message type 0x%04x", msgType)
	}
}

// ---------------------------------------------------------------------------
// Incoming PDU handlers
// ---------------------------------------------------------------------------

// processFormatList handles cbFormatList from the server (the server's
// clipboard content changed). If CF_UNICODETEXT is among the advertised
// formats we immediately request the data.
func (c *clipboardChannel) processFormatList(data []byte) {
	// Acknowledge the format list immediately.
	c.sendPDU(cbFormatListResponse, cbResponseOK, nil)

	// Check whether CF_UNICODETEXT is available.
	r := bytes.NewReader(data)
	hasText := false
	for r.Len() >= 4 {
		fmtID, err := core.ReadUInt32LE(r)
		if err != nil {
			break
		}
		if fmtID == cfUnicodeText {
			hasText = true
		}
		// Skip the format name (null-terminated UTF-16LE string).
		for r.Len() >= 2 {
			ch, err := core.ReadUint16LE(r)
			if err != nil || ch == 0 {
				break
			}
		}
	}
	if hasText {
		c.sendFormatDataRequest(cfUnicodeText)
	}
}

// processFormatDataResponse handles cbFormatDataResponse — the server is
// sending us the clipboard content we requested.
func (c *clipboardChannel) processFormatDataResponse(flags uint16, data []byte) {
	if flags&cbResponseFail != 0 || len(data) == 0 {
		return
	}
	// Remove the UTF-16LE null terminator if present.
	if len(data) >= 2 && data[len(data)-1] == 0 && data[len(data)-2] == 0 {
		data = data[:len(data)-2]
	}
	text := core.UnicodeDecode(data)
	if text == "" {
		return
	}
	c.mu.Lock()
	c.lastContent = text
	c.mu.Unlock()
	clipboard.WriteAll(text)
}

// processFormatDataRequest handles cbFormatDataRequest — the server wants to
// read our local clipboard.
func (c *clipboardChannel) processFormatDataRequest(data []byte) {
	if len(data) < 4 {
		c.sendPDU(cbFormatDataResponse, cbResponseFail, nil)
		return
	}
	r := bytes.NewReader(data)
	fmtID, _ := core.ReadUInt32LE(r)
	if fmtID != cfUnicodeText {
		c.sendPDU(cbFormatDataResponse, cbResponseFail, nil)
		return
	}
	text, err := clipboard.ReadAll()
	if err != nil {
		c.sendPDU(cbFormatDataResponse, cbResponseFail, nil)
		return
	}
	if !isTextContent(text) {
		c.sendPDU(cbFormatDataResponse, cbResponseFail, nil)
		return
	}
	encoded := core.UnicodeEncode(text)
	// Append UTF-16LE null terminator.
	encoded = append(encoded, 0, 0)
	c.sendFormatDataResponse(encoded)
}

// ---------------------------------------------------------------------------
// Outgoing PDU helpers
// ---------------------------------------------------------------------------

// sendPDU constructs a CLIPRDR PDU (8-byte header + data) and sends it over
// the virtual channel.
func (c *clipboardChannel) sendPDU(msgType, msgFlags uint16, data []byte) {
	buf := &bytes.Buffer{}
	core.WriteUInt16LE(msgType, buf)
	core.WriteUInt16LE(msgFlags, buf)
	core.WriteUInt32LE(uint32(len(data)), buf)
	buf.Write(data)
	c.sender.SendToChannel(plugin.CLIPRDR_SVC_CHANNEL_NAME, buf.Bytes())
}

// sendCapabilities advertises client CLIPRDR capabilities to the server.
func (c *clipboardChannel) sendCapabilities() {
	buf := &bytes.Buffer{}
	// cCapabilitiesSets = 1
	core.WriteUInt16LE(1, buf)
	// pad
	core.WriteUInt16LE(0, buf)
	// General capability set
	core.WriteUInt16LE(cbCapsTypeGeneral, buf)    // capabilitySetType
	core.WriteUInt16LE(12, buf)                    // lengthCapability
	core.WriteUInt32LE(cbCapsVersion2, buf)        // version
	core.WriteUInt32LE(cbUseLongFormatNames, buf)  // generalFlags
	c.sendPDU(cbClipCaps, 0, buf.Bytes())
}

// sendFormatList advertises CF_UNICODETEXT as the only available format.
func (c *clipboardChannel) sendFormatList() {
	buf := &bytes.Buffer{}
	core.WriteUInt32LE(cfUnicodeText, buf) // format ID
	// Long format name: empty null-terminated UTF-16LE string (just 2 zero bytes).
	core.WriteUInt16LE(0, buf)
	c.sendPDU(cbFormatList, 0, buf.Bytes())
}

// sendFormatDataRequest asks the server for clipboard data in the given
// format.
func (c *clipboardChannel) sendFormatDataRequest(formatID uint32) {
	buf := &bytes.Buffer{}
	core.WriteUInt32LE(formatID, buf)
	c.sendPDU(cbFormatDataRequest, 0, buf.Bytes())
}

// sendFormatDataResponse sends local clipboard content to the server.
func (c *clipboardChannel) sendFormatDataResponse(data []byte) {
	c.sendPDU(cbFormatDataResponse, cbResponseOK, data)
}

// ---------------------------------------------------------------------------
// Clipboard polling
// ---------------------------------------------------------------------------

// pollClipboard periodically checks the local system clipboard for changes
// and, when a change is detected, advertises the new content to the server
// via a format list PDU.
func (c *clipboardChannel) pollClipboard() {
	c.mu.Lock()
	c.lastContent, _ = clipboard.ReadAll()
	c.mu.Unlock()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopPoll:
			return
		case <-ticker.C:
			content, err := clipboard.ReadAll()
			if err != nil {
				continue
			}
			c.mu.Lock()
			changed := content != c.lastContent
			if changed {
				c.lastContent = content
			}
			c.mu.Unlock()
			if changed && isTextContent(content) {
				c.sendFormatList()
			}
		}
	}
}

// stop terminates the clipboard polling goroutine.
func (c *clipboardChannel) stop() {
	select {
	case <-c.stopPoll:
		// already closed
	default:
		close(c.stopPoll)
	}
}
