// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"bytes"
	"io"
)

func newReader(data []byte) io.Reader { return bytes.NewReader(data) }
