# Go: примеры удачных и неудачных решений

Примеры учебные: внешние типы (`Service`, `Repository`, `ErrNotFound` и др.) условны. В реальном проекте нужно подставлять существующие реализации. «Плохо» означает неподходящее решение **для показанного контекста**, а не универсальный запрет.

## 1. Ошибки: сохраняй причину

**Неудачно — теряется исходная ошибка:**

```go
func (s *Service) Load(ctx context.Context, id int64) (*User, error) {
    user, err := s.repo.Get(ctx, id)
    if err != nil {
        return nil, errors.New("load failed")
    }
    return user, nil
}
```

**Лучше — ошибка оборачивается:**

```go
func (s *Service) Load(ctx context.Context, id int64) (*User, error) {
    user, err := s.repo.Get(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("load user %d: %w", id, err)
    }
    return user, nil
}
```

Внешний контракт может требовать вместо исходной ошибки безопасную публичную категорию — тогда преобразуй осмысленно.

## 2. Небольшой интерфейс на стороне потребителя

**Неудачно:** компоненту нужна только выборка, но он зависит от всех методов большого интерфейса.

```go
type Users interface {
    Get(context.Context, int64) (User, error)
    Create(context.Context, User) error
    Delete(context.Context, int64) error
    List(context.Context) ([]User, error)
}
```

**Лучше:**

```go
type UserGetter interface {
    Get(context.Context, int64) (User, error)
}

type Notifier struct { users UserGetter }
```

Если подмена не нужна, конкретный тип может быть ещё понятнее.

## 3. Typed nil интерфейс

**Неудачно:**

```go
func validate() error {
    var e *ValidationError
    return e // интерфейс error НЕ равен nil
}
```

**Лучше:**

```go
func validate() error {
    return nil // при успехе возвращай настоящий nil
}
```

## 4. Table-driven + t.Parallel + t.Context + require + expect

Условный пример для `testify/mock` (предполагается `MockRepository.Get(ctx, id)` и `NewService(repo)`).

**Неудачно — тест ветвится по имени кейса, а моки создаются вне подтестов:**

```go
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        if tt.name == "not found" {
            repo.On("Get", mock.Anything, tt.id).Return(User{}, ErrNotFound)
        }
        // ...
    })
}
```

**Лучше — кейсы самодостаточны:**

```go
func TestService_Get(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name    string
        id      int64
        expect  func(*MockRepository)
        want    User
        wantErr error
    }{
        {
            name: "found", id: 1,
            expect: func(m *MockRepository) {
                m.On("Get", mock.Anything, int64(1)).
                    Return(User{ID: 1}, nil).Once()
            },
            want: User{ID: 1},
        },
        {
            name: "missing", id: 2,
            expect: func(m *MockRepository) {
                m.On("Get", mock.Anything, int64(2)).
                    Return(User{}, ErrNotFound).Once()
            },
            wantErr: ErrNotFound,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            repo := new(MockRepository)
            repo.Test(t)
            tt.expect(repo)
            t.Cleanup(func() { repo.AssertExpectations(t) })

            got, err := NewService(repo).Get(t.Context(), tt.id)
            if tt.wantErr != nil {
                require.ErrorIs(t, err, tt.wantErr)
                return
            }
            require.NoError(t, err)
            require.Equal(t, tt.want, got)
        })
    }
}
```

`require` вызывается в тестовой горутине, моки изолированы по кейсам. Для Go до 1.24 `t.Context()` недоступен.

## 5. Правильные проверки ошибок

**Неудачно:**

```go
require.True(t, strings.Contains(err.Error(), "already exists"))
```

**Лучше, если проверяется текст:**

```go
require.ErrorContains(t, err, "already exists")
```

**Лучше, если проверяется категория:**

```go
require.ErrorIs(t, err, ErrAlreadyExists)
```

Для проверки типа используй `require.ErrorAs`, для результата — `require.Equal(t, want, got)` и другие специальные функции.

## 6. Асинхронные проверки

**Неудачно — ожидание фиксировано:**

```go
time.Sleep(time.Second)
require.Equal(t, 1, worker.ProcessedCount())
```

**Лучше — ожидание условия:**

```go
require.Eventually(t, func() bool {
    return worker.ProcessedCount() == 1 // метод обязан быть потокобезопасен
}, 3*time.Second, 10*time.Millisecond)
```

Для нескольких условий `require.EventuallyWithT` использует `assert.CollectT`, и внутри callback следует применять `assert.*`, а не `require.*`. При возможности предпочитай явную синхронизацию или тестовые часы.

## 7. Проверяй не только действия, но и отсутствие действий

**Неудачно:** проверка только `require.ErrorIs(t, err, ErrInvalidInput)`.

**Лучше:** после ошибки валидации дополнительно проверь, что запись не запускалась. Для `testify/mock` в table-driven тесте можно зарегистрировать проверку через `t.Cleanup` внутри `expect` конкретного кейса:

```go
expect: func(m *MockRepository) {
    t.Cleanup(func() {
        m.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
    })
},
```

Это **фрагмент тела подтеста**, поэтому `t` здесь — объект соответствующего подтеста. Проверка выполнится после тестируемого действия. Если в проекте приняты отдельные постпроверки (`verify`) или другой mock-фреймворк, следуй существующему подходу. Не пытайся вызвать `AssertNotCalled` до операции — такой тест не обнаружит последующий запрещённый вызов.

## 8. SQL: параметры и контекст

**Неудачно:**

```go
q := fmt.Sprintf("SELECT id FROM users WHERE name = '%s'", name)
row := db.QueryRow(q)
```

**Лучше для PostgreSQL:**

```go
row := db.QueryRowContext(ctx,
    "SELECT id FROM users WHERE name = $1", name)
```

Проверяй результат `Scan`, для многострочных запросов — `rows.Err()`, корректно закрывай `Rows`. Другие БД могут использовать иные placeholders.

## 9. Транзакция: успешный запрос ещё не означает успешную операцию

**Неудачно:** игнорировать ошибку `Commit()`.

**Лучше:**

```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return fmt.Errorf("begin: %w", err) }
defer tx.Rollback()

if _, err := tx.ExecContext(ctx, query, args...); err != nil {
    return fmt.Errorf("write: %w", err)
}
if err := tx.Commit(); err != nil {
    return fmt.Errorf("commit: %w", err)
}
return nil
```

При сетевой ошибке Commit фактический исход может быть неизвестен; повтор должен учитывать идемпотентность.

## 10. Контекст и параллельность

**Неудачно:**

```go
func TestConfig(t *testing.T) {
    t.Parallel()
    t.Setenv("APP_MODE", "test") // несовместимо с параллельным тестом
}
```

**Лучше:** не использовать `t.Parallel` для сценария с `t.Setenv`, либо передавать конфигурацию явно и тестировать параллельно без изменения process-wide окружения.

## 11. Управление горутиной и отменой

**Неудачно:** запуск `go worker.Run(context.Background())` без владельца и способа дождаться завершения.

**Лучше:** передать отменяемый контекст от вызывающего компонента и обеспечить `cancel` + ожидание завершения. В тесте использовать дочерний `context.WithCancel(t.Context())`, но убедиться, что worker успел стартовать, если проверяется отмена уже работающей горутины.

## 12. Формулировка проверки перед коммитом

```bash
make lint
make test
# Для изменений конкурентного кода, если это ещё не включено в Makefile:
go test -race ./...
```

Не дублируй проверки без необходимости и не запускай потенциально разрушительные интеграционные тесты против production.
