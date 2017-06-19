// Useful utility functions
package common

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"time"
)

// Generate a stable SHA256 for a commit.
func CreateCommitID(c CommitEntry) string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("tree %s\n", c.Tree.ID))
	if c.Parent != "" {
		b.WriteString(fmt.Sprintf("parent %s\n", c.Parent))
	}
	b.WriteString(fmt.Sprintf("author %s <%s> %v\n", c.AuthorName, c.AuthorEmail,
		c.Timestamp.Format(time.UnixDate)))
	if c.CommitterEmail != "" {
		b.WriteString(fmt.Sprintf("committer %s <%s> %v\n", c.CommitterName, c.CommitterEmail,
			c.Timestamp.Format(time.UnixDate)))
	}
	b.WriteString("\n" + c.Message)
	b.WriteByte(0)
	s := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(s[:])
}

// Generate the SHA256 for a tree.
// Tree entry structure is:
// * [ entry type ] [ sha256 ] [ db name ] [ last modified (timestamp) ] [ db size (bytes) ]
func CreateDBTreeID(entries []DBTreeEntry) string {
	var b bytes.Buffer
	for _, j := range entries {
		b.WriteString(string(j.EntryType))
		b.WriteByte(0)
		b.WriteString(j.Sha256)
		b.WriteByte(0)
		b.WriteString(j.Name)
		b.WriteByte(0)
		b.WriteString(j.Last_Modified.Format(time.RFC3339))
		b.WriteByte(0)
		b.WriteString(fmt.Sprintf("%d\n", j.Size))
	}
	s := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(s[:])
}

// Look for the next child fork in a fork tree
func nextChild(loggedInUser string, rawListPtr *[]ForkEntry, outputListPtr *[]ForkEntry, forkTrailPtr *[]int, iconDepth int) ([]ForkEntry, []int, bool) {
	// TODO: This approach feels half arsed.  Maybe redo it as a recursive function instead?

	// Resolve the pointers
	rawList := *rawListPtr
	outputList := *outputListPtr
	forkTrail := *forkTrailPtr

	// Grab the last database ID from the fork trail
	parentID := forkTrail[len(forkTrail)-1:][0]

	// Scan unprocessed rows for the first child of parentID
	numResults := len(rawList)
	for j := 1; j < numResults; j++ {
		// Skip already processed entries
		if rawList[j].Processed == false {
			if rawList[j].ForkedFrom == parentID {
				// * Found a fork of the parent *

				// Set the icon list for display in the browser
				for k := 0; k < iconDepth; k++ {
					rawList[j].IconList = append(rawList[j].IconList, SPACE)
				}
				rawList[j].IconList = append(rawList[j].IconList, END)

				// If the database is no longer public, then use placeholder details instead
				if !rawList[j].Public && loggedInUser != rawList[j].Owner {
					rawList[j].DBName = "private database"
				}

				// Add this database to the output list
				outputList = append(outputList, rawList[j])

				// Append this database ID to the fork trail
				forkTrail = append(forkTrail, rawList[j].ID)

				// Mark this database entry as processed
				rawList[j].Processed = true

				// Indicate a child fork was found
				return outputList, forkTrail, true
			}
		}
	}

	// Indicate no child fork was found
	return outputList, forkTrail, false
}

// Generate a random string
func RandomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	const alphaNum = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomString := make([]byte, length)
	for i := range randomString {
		randomString[i] = alphaNum[rand.Intn(len(alphaNum))]
	}

	return string(randomString)
}
