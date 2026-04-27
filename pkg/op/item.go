package op

type Item struct {
	ID           string        `json:"id,omitempty"`
	Title        string        `json:"title,omitempty"`
	Version      int           `json:"version,omitempty"`
	Vault        Vault         `json:"vault,omitempty"`
	Category     string        `json:"category,omitempty"`
	LastEditedBy string        `json:"last_edited_by,omitempty"`
	CreatedAt    string        `json:"created_at,omitempty"`
	UpdatedAt    string        `json:"updated_at,omitempty"`
	Sections     []ItemSection `json:"sections,omitempty"`
	Fields       []ItemField   `json:"fields,omitempty"`
}

type ItemSection struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

const (
	ItemIDNotesPlain    = "notesPlain"
	ItemFieldTypeString = "STRING"

	ItemPurposeNotes = "NOTES"

	ItemLabelNotesPlain = "notesPlain"
)

type ItemField struct {
	ID        string       `json:"id,omitempty"`
	Type      string       `json:"type,omitempty"`
	Purpose   string       `json:"purpose,omitempty"`
	Label     string       `json:"label,omitempty"`
	Value     string       `json:"value,omitempty"`
	Reference string       `json:"reference,omitempty"`
	Section   *ItemSection `json:"section,omitempty"`
}
