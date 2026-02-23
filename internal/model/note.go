package model

// FilterNotesRequest - запрос на фильтрацию списка запрашиваемых заметок.
// UserID передается в токене.
type FilterNotesRequest struct {
	UserID  int   `json:"-"`
	SpaceID int   `json:"space_id"`
	NoteIDs []int `json:"note_ids"`
}

// NoteAccessInfo - информация о доступе пользователя к заметке.
type NoteAccessInfo struct {
	CanRead bool `json:"can_read"`
	CanEdit bool `json:"can_edit"` // может ли пользователь редактировать заметку
}

// FilterNotesResponse - ответ на запрос FilterNotesRequest.
// Объект-словарь, где ключ — ID заметки (строкой), значение — объект с флагом CanEdit.
type FilterNotesResponse struct {
	Notes map[int]NoteAccessInfo `json:"notes"`
}
