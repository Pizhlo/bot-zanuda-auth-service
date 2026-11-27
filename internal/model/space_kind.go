package model

// SpaceKind задаёт тип будущего пространства в проверке создания и тип синхронизированного пространства в OpenFGA.
// Значения строчные; старый SpaceType с PERSONAL/SHARED относится к SQL-модели
// и сохраняет свой контракт. Преобразование между ними должно быть явным.
type SpaceKind string

const (
	// SpaceKindPersonal — встроенное пространство: без участников и удаления.
	SpaceKindPersonal SpaceKind = "personal"
	// SpaceKindShared — совместное пространство с ролями участников.
	SpaceKindShared SpaceKind = "shared"
	// AttributeSpaceType — атрибут синхронизации, задающий тип при создании.
	AttributeSpaceType = "space_type"
	// RelationSpaceCreated связывает UUID пространства с единственной записью создания.
	RelationSpaceCreated = "created"
	// RelationHasPersonalSpace связывает зарегистрированного пользователя с ним самим один раз.
	RelationHasPersonalSpace = "has_personal_space"
)

// Valid сообщает, можно ли использовать значение как маркер типа в OpenFGA.
// Отсутствующий или неизвестный тип не должен превращаться в shared по умолчанию.
func (k SpaceKind) Valid() bool {
	return k == SpaceKindPersonal || k == SpaceKindShared
}
