//-----------------------------------------------------------------------------
// Copyright (c) 2021-present Detlef Stern
//
// This file is part of zettelstore-client.
//
// Zettelstore client is licensed under the latest version of the EUPL
// (European Union Public License). Please see file LICENSE.txt for your rights
// and obligations under this license.
//-----------------------------------------------------------------------------

// Package api contains common definitions used for client and server.
package api

// ZettelID contains the identifier of a zettel. It is a string with 14 digits.
type ZettelID string

// InvalidZID is an invalif zettel identifier
const InvalidZID = ""

// IsValid returns true, if the idenfifier contains 14 digits.
func (zid ZettelID) IsValid() bool {
	if len(zid) != 14 {
		return false
	}
	for i := 0; i < 14; i++ {
		ch := zid[i]
		if ch < '0' || '9' < ch {
			return false
		}
	}
	return true
}

// ZettelMeta is a map containg the metadata of a zettel.
type ZettelMeta map[string]string

// ZettelRights is an integer that encode access rights for a zettel.
type ZettelRights uint8

// Values for ZettelRights, can be or-ed
const (
	ZettelCanNone   ZettelRights = 1 << iota
	ZettelCanCreate              // Current user is allowed to create a new zettel
	ZettelCanRead                // Requesting user is allowed to read the zettel
	ZettelCanWrite               // Requesting user is allowed to update the zettel
	ZettelCanRename              // Requesting user is allowed to provide the zettel with a new identifier
	ZettelCanDelete              // Requesting user is allowed to delete the zettel
)

// ZidJSON contains the identifier data of a zettel.
type ZidJSON struct {
	ID ZettelID `json:"id"`
}

// MetaJSON contains the metadata of a zettel.
type MetaJSON struct {
	Meta   ZettelMeta   `json:"meta"`
	Rights ZettelRights `json:"rights"`
}

// ZidMetaJSON contains the identifier and the metadata of a zettel.
type ZidMetaJSON struct {
	ID     ZettelID     `json:"id"`
	Meta   ZettelMeta   `json:"meta"`
	Rights ZettelRights `json:"rights"`
}

// ZidMetaRelatedList contains identifier/metadata of a zettel and the same for related zettel
type ZidMetaRelatedList struct {
	ID     ZettelID      `json:"id"`
	Meta   ZettelMeta    `json:"meta"`
	Rights ZettelRights  `json:"rights"`
	List   []ZidMetaJSON `json:"list"`
}

// ZettelData contains all data for a zettel.
type ZettelData struct {
	Meta     ZettelMeta `json:"meta"`
	Encoding string     `json:"encoding"`
	Content  string     `json:"content"`
}

// ZettelJSON contains all data for a zettel, the identifier, the metadata, and the content.
type ZettelJSON struct {
	ID       ZettelID     `json:"id"`
	Meta     ZettelMeta   `json:"meta"`
	Encoding string       `json:"encoding"`
	Content  string       `json:"content"`
	Rights   ZettelRights `json:"rights"`
}

// ZettelContentJSON contains all elements to transfer the content of a zettel.
type ZettelContentJSON struct {
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

// ZettelListJSON contains data for a zettel list.
type ZettelListJSON struct {
	Query string        `json:"query"`
	Human string        `json:"human"`
	List  []ZidMetaJSON `json:"list"`
}

// MapMeta maps metadata keys to list of metadata.
type MapMeta map[string][]ZettelID

// MapListJSON specifies the map of metadata key to list of metadata that contains the key.
type MapListJSON struct {
	Map MapMeta `json:"map"`
}
