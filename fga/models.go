// Package fgamodel содержит модели перехода и строгой авторизации пространств.
package fgamodel

import _ "embed" // Модели доступны инструменту переноса и тестам независимо от cwd.

// Strict содержит модель с защитой личных пространств.
//
//go:embed auth-model.fga
var Strict string

// Transition добавляет тип пространства к прежним отношениям для подготовки данных.
//
//go:embed transition-model.fga
var Transition string
