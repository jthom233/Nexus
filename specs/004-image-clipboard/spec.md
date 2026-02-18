# Feature Specification: Image Clipboard Support for RDP

**Feature Branch**: `004-image-clipboard`
**Created**: 2026-02-17
**Status**: Draft
**Input**: User description: "Add full image clipboard support for RDP sessions — copy/paste images between local system and remote desktop via CLIPRDR CF_DIB format"

## Clarifications

### Session 2026-02-17

- Q: With paned sessions (003), which session receives clipboard image data? → A: Focused/active session only, consistent with existing text clipboard behavior.
- Q: What feedback does the user see when an image clipboard transfer fails? → A: Silent failure with debug-level log entry. No dialog or notification — the paste simply doesn't appear, matching native RDP client UX.
- Q: Should VNC sessions also get image clipboard support? → A: RDP only for this feature. VNC uses a different clipboard protocol and would be a separate feature.
- Q: Should large image transfers show a progress indicator? → A: No progress indicator. Keep it simple — just ensure no hang or freeze.
- Q: What is explicitly out of scope? → A: File clipboard (CF_HDROP), drag-and-drop image transfer, and metafile formats (CF_ENHMETAFILE, CF_METAFILEPICT) are all out of scope.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Paste Local Image into RDP Session (Priority: P1)

A user takes a screenshot on their local machine (e.g., Print Screen, Snipping Tool, or screenshot utility), then switches to their active RDP session and pastes (Ctrl+V). The image appears in whatever application has focus on the remote desktop — a Word document, Paint, Slack, a browser upload field, etc. — just as if the screenshot had been taken on the remote machine.

**Why this priority**: This is the most common clipboard image workflow — users constantly paste screenshots, diagrams, and cropped images from their local machine into remote applications. Without this, users must save the image to a file, transfer it, then open it remotely. This is the core value of image clipboard.

**Independent Test**: Can be fully tested by taking a screenshot locally, switching to the RDP session, and pasting into Paint or Notepad++. Delivers the core "paste image into remote" workflow.

**Acceptance Scenarios**:

1. **Given** a user has copied a PNG screenshot to the local clipboard, **When** they press Ctrl+V in a remote application that accepts images, **Then** the image appears at full fidelity in the remote application.
2. **Given** a user has copied an image from a local application (e.g., browser "Copy Image"), **When** they paste into a remote application, **Then** the image is transferred and displayed correctly.
3. **Given** a user has copied text to the local clipboard (not an image), **When** they paste into the RDP session, **Then** the existing text clipboard functionality works unchanged.

---

### User Story 2 - Copy Image from RDP Session to Local Machine (Priority: P2)

A user copies an image inside the RDP session (e.g., right-click "Copy" on an image in a browser, or copies a chart from Excel on the remote desktop), then switches to a local application and pastes. The image appears locally as expected.

**Why this priority**: The reverse direction completes the bidirectional clipboard story. While less frequent than local-to-remote, it's essential for users who generate content remotely (charts, screenshots of remote-only applications) and need to bring it to their local workflow.

**Independent Test**: Can be tested by copying an image inside the RDP session (e.g., right-click an image in a browser → Copy Image), switching to a local image editor, and pasting. Delivers the "bring remote images local" workflow.

**Acceptance Scenarios**:

1. **Given** a user copies an image inside the RDP session, **When** they switch to a local application and paste, **Then** the image appears locally at full fidelity.
2. **Given** a user copies a screenshot inside the RDP session (e.g., remote Print Screen), **When** they paste locally, **Then** the screenshot is available as an image.
3. **Given** a user copies text inside the RDP session, **When** they paste locally, **Then** the existing text clipboard functionality works unchanged.

---

### User Story 3 - Large Image Handling (Priority: P3)

A user copies a very large image (e.g., a high-resolution screenshot of a multi-monitor setup, a large diagram, or a photo) to the clipboard and pastes it into the RDP session. The transfer completes without timeout or corruption, though it may take slightly longer than small images.

**Why this priority**: Large images are a realistic edge case — multi-monitor screenshots, high-DPI captures, and large diagrams are common in enterprise workflows. Graceful handling prevents user frustration and data loss.

**Independent Test**: Can be tested by copying a 4K+ resolution screenshot (>10MB) and pasting into the RDP session. Verify the image arrives intact without truncation or corruption.

**Acceptance Scenarios**:

1. **Given** a user has a large image (>5MB) on the clipboard, **When** they paste into the RDP session, **Then** the image transfers completely (no truncation) and appears correctly.
2. **Given** a very large image exceeds the maximum transfer size, **When** the user attempts to paste, **Then** the system gracefully degrades (no crash, no hang) and the user is not left waiting indefinitely.

---

### Edge Cases

- What happens when the clipboard contains both text and image representations? The system should advertise both CF_UNICODETEXT and CF_DIB formats, letting the remote application choose which format it wants.
- What happens when the remote application requests an image format the system doesn't support (e.g., CF_METAFILEPICT, CF_ENHMETAFILE)? The system should respond with CB_RESPONSE_FAIL for unsupported formats while still serving supported ones. Metafile formats are explicitly out of scope.
- What happens when the clipboard image is in a format other than PNG (e.g., JPEG, BMP, WebP)? The system should convert any local image format to the Windows DIB format required by the CLIPRDR protocol.
- What happens when the clipboard changes rapidly (e.g., user copies multiple things in quick succession)? The system should debounce and only advertise the latest clipboard content.
- What happens during the image transfer if the RDP session disconnects? The transfer should fail silently (debug log only) without crashing the GUI or leaving the clipboard in a broken state. The paste simply doesn't appear.
- What happens on platforms where the clipboard API doesn't distinguish image from text content? The system should use content-type detection (magic bytes, MIME type queries) to determine the clipboard content type.
- What happens when multiple RDP sessions are open (paned sessions)? Image clipboard targets only the focused/active session, consistent with text clipboard behavior.
- What happens if the user pastes a file or drag-and-drops an image? File clipboard (CF_HDROP) and drag-and-drop are out of scope for this feature.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST detect when the local clipboard contains image content (PNG, JPEG, BMP, GIF, WebP) and advertise CF_DIB format to the RDP server via CLIPRDR.
- **FR-002**: System MUST continue to advertise CF_UNICODETEXT when the clipboard contains text, preserving existing text clipboard behavior.
- **FR-003**: When the clipboard contains both text and image representations, the system MUST advertise both CF_UNICODETEXT and CF_DIB, allowing the remote application to choose.
- **FR-004**: System MUST convert local image clipboard data to Windows DIB format (BITMAPINFOHEADER + bottom-up 32-bit BGRA pixel data) when the RDP server requests CF_DIB.
- **FR-005**: System MUST handle CF_DIB data received from the RDP server by converting it to a platform-appropriate image format and placing it on the local clipboard.
- **FR-006**: System MUST support images up to 20MB in clipboard transfer size without truncation or timeout.
- **FR-007**: System MUST respond with CB_RESPONSE_FAIL for clipboard format requests it cannot fulfill, rather than sending corrupt data.
- **FR-008**: System MUST not crash or hang during clipboard transfer failures (network interruption, format conversion errors, oversized images). Failures are silent with debug-level logging only — no user-facing dialogs or notifications.
- **FR-009**: System MUST work on both Linux (Wayland and X11) and Windows for local clipboard access.
- **FR-010**: System MUST debounce rapid clipboard changes, advertising only the latest content state to the RDP server.
- **FR-011**: Image clipboard MUST target only the focused/active RDP session when multiple sessions are open (paned sessions).
- **FR-012**: Image clipboard applies to RDP sessions only. VNC sessions are out of scope for this feature.
- **FR-013**: File clipboard (CF_HDROP), drag-and-drop, and metafile formats (CF_ENHMETAFILE, CF_METAFILEPICT) are explicitly out of scope and MUST NOT be implemented as part of this feature.

### Key Entities

- **ClipboardContent**: Represents the current local clipboard state. Has a content type (text, image, or both), raw data, and detected MIME type. Used to determine which CLIPRDR formats to advertise.
- **DIBImage**: The Windows Device Independent Bitmap representation. Contains a BITMAPINFOHEADER (width, height, bit depth, compression) and pixel data in bottom-up BGRA byte order. This is the wire format for image clipboard data in the RDP CLIPRDR channel.
- **FormatList**: The set of clipboard formats advertised to or received from the RDP server. Maps format IDs (CF_UNICODETEXT=13, CF_DIB=8) to availability. Determines which data the remote side can request.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can copy a screenshot locally and paste it as an image in a remote application within 2 seconds for images under 2MB.
- **SC-002**: Users can copy an image from a remote application and paste it locally, with the image appearing intact and at correct dimensions.
- **SC-003**: Images up to 20MB transfer without corruption or truncation.
- **SC-004**: Existing text clipboard copy/paste continues to work identically — no regression.
- **SC-005**: Clipboard operations complete without any user-visible errors or hangs, even when transfer fails (graceful degradation).
- **SC-006**: Image clipboard works on both Linux (Wayland/X11) and Windows platforms.

## Assumptions

- The RDP server supports CF_DIB format in the CLIPRDR channel, which is standard for all modern Windows RDP servers.
- Local clipboard image access requires platform-specific code: `wl-paste --type image/png` on Wayland, `xclip -t image/png` on X11, and Win32 clipboard API on Windows. The `atotto/clipboard` library is text-only and will need to be supplemented for image support.
- The maximum practical clipboard image size is bounded by available memory and network throughput. The 20MB limit is a reasonable upper bound for typical use cases.
- CF_DIB uses 32-bit BGRA pixel format with a 40-byte BITMAPINFOHEADER. No compression (BI_RGB). This is the most widely supported format across Windows applications.
