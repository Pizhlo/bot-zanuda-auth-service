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

// VisibilityType - уровень видимости заметки.
type VisibilityType string

const (
	// VisibilityTypeSpace - уровень видимости на все пространство.
	// Запись доступна всем, если не указано иначе правами.
	VisibilityTypeSpace VisibilityType = "SPACE"
	// VisibilityTypePrivateToAuthor - уровень видимости, при котором запись видна только автору.
	VisibilityTypePrivateToAuthor VisibilityType = "PRIVATE_TO_AUTHOR"
	// VisibilityTypeCustom - уровень видимости, при котором доступ к записи установлен индивидуально:
	// каким-то пользователям можно смотреть, каким-то - нет. Информация об этом лежит в таблице notes.notes_acl.
	VisibilityTypeCustom VisibilityType = "CUSTOM"
)

// NoteVisibility - модель для хранения информации об уровне видимости заметки.
type NoteVisibility struct {
	ID         int            // id заметки
	Visibility VisibilityType // уровень видимости: SPACE, PRIVATE_TO_AUTHOR, CUSTOM
}
