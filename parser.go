package irc

import (
	"bytes"
	"strings"
	"unicode"
)

var tagDecodeSlashMap = map[rune]rune{
	':':  ';',
	's':  ' ',
	'\\': '\\',
	'r':  '\r',
	'n':  '\n',
}

var tagEncodeMap = map[rune]string{
	';':  "\\:",
	' ':  "\\s",
	'\\': "\\\\",
	'\r': "\\r",
	'\n': "\\n",
}

// TagValue represents the value of a tag.
type TagValue string

// ParseTagValue parses a TagValue from the connection. If you need to
// set a TagValue, you probably want to just set the string itself, so
// it will be encoded properly.
func ParseTagValue(v string) TagValue {
	ret := &bytes.Buffer{}

	input := bytes.NewBufferString(v)

	for {
		c, _, err := input.ReadRune()
		if err != nil {
			break
		}

		if c == '\\' {
			c2, _, err := input.ReadRune()
			if err != nil {
				ret.WriteRune(c)
				break
			}

			if replacement, ok := tagDecodeSlashMap[c2]; ok {
				ret.WriteRune(replacement)
			} else {
				ret.WriteRune(c)
				ret.WriteRune(c2)
			}
		} else {
			ret.WriteRune(c)
		}
	}

	return TagValue(ret.String())
}

// Encode converts a TagValue to the format in the connection.
func (v TagValue) Encode() string {
	ret := &bytes.Buffer{}

	for _, c := range v {
		if replacement, ok := tagEncodeMap[c]; ok {
			ret.WriteString(replacement)
		} else {
			ret.WriteRune(c)
		}
	}

	return ret.String()
}

// Tags represents the IRCv3 message tags.
type Tags map[string]TagValue

// ParseTags takes a tag string and parses it into a tag map. It will
// always return a tag map, even if there are no valid tags.
func ParseTags(line string) Tags {
	ret := Tags{}

	tags := strings.Split(line, ";")
	for _, tag := range tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) < 2 {
			ret[parts[0]] = ""
			continue
		}

		ret[parts[0]] = ParseTagValue(parts[1])
	}

	return ret
}

// GetTag is a convenience method to look up a tag in the map.
func (t Tags) GetTag(key string) (string, bool) {
	ret, ok := t[key]
	return string(ret), ok
}

// Copy will create a new copy of all IRC tags attached to this
// message.
func (t Tags) Copy() Tags {
	ret := Tags{}

	for k, v := range t {
		ret[k] = v
	}

	return ret
}

// String ensures this is stringable
func (t Tags) String() string {
	buf := &bytes.Buffer{}

	for k, v := range t {
		buf.WriteByte(';')
		buf.WriteString(k)
		if v != "" {
			buf.WriteByte('=')
			buf.WriteString(v.Encode())
		}
	}

	// We don't need the first byte because that's an extra ';'
	// character.
	buf.ReadByte()

	return buf.String()
}

// Prefix represents the prefix of a message, generally the user who sent it
type Prefix struct {
	// Name will contain the nick of who sent the message, the
	// server who sent the message, or a blank string
	Name string

	// User will either contain the user who sent the message or a blank string
	User string

	// Host will either contain the host of who sent the message or a blank string
	Host string
}

// ParsePrefix takes an identity string and parses it into an
// identity struct. It will always return an Prefix struct and never
// nil.
func ParsePrefix(line string) *Prefix {
	// Start by creating an Prefix with nothing but the host
	id := &Prefix{
		Name: line,
	}

	idx := strings.IndexRune(id.Name, '@')
	if idx > 0 {
		id.Name, id.Host = id.Name[:idx], id.Name[idx+1:]
	}

	idx = strings.IndexRune(id.Name, '!')
	if idx > 0 {
		id.Name, id.User = id.Name[:idx], id.Name[idx+1:]
	}

	return id
}

// Copy will create a new copy of an Prefix
func (p *Prefix) Copy() *Prefix {
	if p == nil {
		return nil
	}

	newPrefix := &Prefix{}

	*newPrefix = *p

	return newPrefix
}

// String ensures this is stringable
func (p *Prefix) String() string {
	buf := &bytes.Buffer{}
	buf.WriteString(p.Name)

	if p.User != "" {
		buf.WriteString("!")
		buf.WriteString(p.User)
	}

	if p.Host != "" {
		buf.WriteString("@")
		buf.WriteString(p.Host)
	}

	return buf.String()
}

// Message represents a line parsed from the server
type Message struct {
	// Each message can have IRCv3 tags
	Tags

	// Each message can have a Prefix
	*Prefix

	// Command is which command is being called.
	Command string

	// Params are all the arguments for the command.
	Params []string
}

// ParseMessage takes a message string (usually a whole line) and
// parses it into a Message struct. This will return nil in the case
// of invalid messages.
func ParseMessage(line string) *Message {
	// Trim the line and make sure we have data
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return nil
	}

	c := &Message{
		Tags:   Tags{},
		Prefix: &Prefix{},
		Params: []string{},
	}

	// 0 == initial
	// 1 == found tags
	// 2 == found prefix
	// 3 == other
	state := 0
	offset := 0
	var idxTokenEnd, idxNextToken int
	for {
		if state == 0 && line[offset] == '@' {
			state = 1
		} else if state <= 1 && line[offset] == ':' {
			state = 2
		} else if line[offset] == ':' {
			c.Params = append(c.Params, line[offset+1:])
			break
		} else {
			state = 3
		}

		idxTokenEnd = strings.IndexFunc(line[offset:], unicode.IsSpace)
		if idxTokenEnd < 0 {
			c.Params = append(c.Params, line[offset:])
			break
		}

		idxNextToken = strings.IndexFunc(line[offset+idxTokenEnd:], isNotSpace)
		/*
			This should never be hit, because this only protects
			against whitespace at the very end, and that was removed
			by strings.TrimSpace at the start of this function.

			if idxNextToken < 0 {
				c.Params = append(c.Params, line[offset:])
				break
			}
		*/

		c.Params = append(c.Params, line[offset:offset+idxTokenEnd])

		offset += idxTokenEnd + idxNextToken
	}

	// If the first param starts with @, we know it contains IRC tags
	if len(c.Params) > 0 && c.Params[0][0] == '@' {
		c.Tags = ParseTags(c.Params[0][1:])
		c.Params = c.Params[1:]
	}

	// If the first param starts with :, we know it contains a Prefix
	if len(c.Params) > 0 && c.Params[0][0] == ':' {
		c.Prefix = ParsePrefix(c.Params[0][1:])
		c.Params = c.Params[1:]
	}

	if len(c.Params) < 1 || len(c.Params[0]) < 1 {
		return nil
	}

	c.Command = c.Params[0]
	c.Params = c.Params[1:]

	return c
}

// Trailing returns the last argument in the Message or an empty string
// if there are no args
func (m *Message) Trailing() string {
	if len(m.Params) < 1 {
		return ""
	}

	return m.Params[len(m.Params)-1]
}

// FromChannel is mostly for PRIVMSG messages (and similar derived messages)
// It will check if the message came from a channel or a person.
func (m *Message) FromChannel() bool {
	if len(m.Params) < 1 || len(m.Params[0]) < 1 {
		return false
	}

	switch m.Params[0][0] {
	case '#', '&':
		return true
	default:
		return false
	}
}

// Copy will create a new copy of an message
func (m *Message) Copy() *Message {
	// Create a new message
	newMessage := &Message{}

	// Copy stuff from the old message
	*newMessage = *m

	// Copy any IRcv3 tags
	newMessage.Tags = m.Tags.Copy()

	// Copy the Prefix
	newMessage.Prefix = m.Prefix.Copy()

	// Copy the Params slice
	newMessage.Params = append(make([]string, 0, len(m.Params)), m.Params...)

	return newMessage
}

// String ensures this is stringable
func (m *Message) String() string {
	buf := &bytes.Buffer{}

	// Write any IRCv3 tags if they exist in the message
	if len(m.Tags) > 0 {
		buf.WriteByte('@')
		buf.WriteString(m.Tags.String())
		buf.WriteByte(' ')
	}

	// Add the prefix if we have one
	if m.Prefix != nil && m.Prefix.Name != "" {
		buf.WriteByte(':')
		buf.WriteString(m.Prefix.String())
		buf.WriteByte(' ')
	}

	// Add the command since we know we'll always have one
	buf.WriteString(m.Command)

	if len(m.Params) > 0 {
		args := m.Params[:len(m.Params)-1]
		trailing := m.Params[len(m.Params)-1]

		if len(args) > 0 {
			buf.WriteByte(' ')
			buf.WriteString(strings.Join(args, " "))
		}

		// If trailing contains a space or starts with a : we
		// need to actually specify that it's trailing.
		if strings.ContainsRune(trailing, ' ') || trailing[0] == ':' {
			buf.WriteString(" :")
		} else {
			buf.WriteString(" ")
		}
		buf.WriteString(trailing)
	}

	return buf.String()
}
