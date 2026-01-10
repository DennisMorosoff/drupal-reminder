---
name: add-tg-feature
description: Implement a new Telegram bot command, callback query, or message handler in Go. Use this when the user asks to "add a feature" or "handle a command".
---

# Adding Telegram Bot Features

When adding new functionality to the Go Telegram bot, follow this Clean Architecture flow:

## 1. Define the Handler
Create a new handler function in the `internal/transport/telegram/handlers` package (or equivalent).
- Function signature must accept `context.Context` and the Update event.
- Parse user input safely.
- **Do not** put business logic here. Call the Service layer.

## 2. Service Layer Interaction
- If logic is complex, define an interface in `internal/service`.
- Inject this service into your Handler struct.
- Example: `func (h *Handler) StartCmd(ctx context.Context, chatID int64) error`

## 3. UI/UX Guidelines (Keyboards)
- If the feature needs buttons, create them in a separate `keyboards` package.
- Use `InlineKeyboardMarkup` for actions, `ReplyKeyboardMarkup` for menus.
- ALWAYS define callback data constants to avoid magic strings.

## 4. Error Handling
- Return errors up the stack.
- Wrap errors using `fmt.Errorf("handler.StartCmd: %w", err)`.
- Log the error in the main middleware, reply to the user with a generic "Something went wrong" message if appropriate.

## 5. Registration
- Don't forget to register the new handler in your Router or Main setup file.
