# Baby Sleep Tracker Bot

Telegram bot for tracking a newborn's sleep in real time and retroactively.

## Features

- Start and end sleep in one tap
- Retroactive buttons:
  - `Начался 5 минут назад`
  - `Начался 10 минут назад`
  - `Начался 15 минут назад`
  - `Начался 30 минут назад`
  - matching buttons for sleep end
- Manual sleep entry and last entry correction
- Reports:
  - latest nap vs yesterday
  - latest nap vs average over 7 and 30 days
  - day / week / month summaries
- Export completed sleep records to CSV (`/export_csv`)
- Two-parent access with invite code
- Reminders:
  - wake window reached
  - current sleep is too long
  - too long without any sleep records
  - custom reminders
- Optional **milestone dates** (life duration from the **birth moment** in the family timezone): push per milestone (`/milestone_notify on|off`, requires `/reminders_on`) and/or a “today’s milestones” block in `/report` and `/day` (`/milestone_report on|off`). Milestones older than 24h are not backfilled when enabling pushes.
- SQLite database for persistent storage

## Quick Start

1. Copy the environment template:

```sh
cp sleepbot.env.example .env
```

2. Fill in `TELEGRAM_BOT_TOKEN`.

3. Build and run:

```sh
go build -o sleepbot .
./sleepbot
```

## Main Commands

- `/start`
- `/help`
- `/invite`
- `/join CODE`
- `/report`
- `/day`
- `/week`
- `/month`
- `/export_csv`
- `/reminders`
- `/milestone_notify on|off`
- `/milestone_report on|off`
- `/settings`
- `/setchild Имя`
- `/settimezone Europe/Moscow`
- `/setbirthdate 16.03.2026 14:30` or `/setbirthdate 16.03.2026` (time in family timezone)
- `/setwake 90`
- `/setmaxsleep 120`
- `/setinactive 240`
- `/addreminder 19:30 Купание`
- `/deletereminder 1`
- `/editlast`
- `/cancel`
- `/silent_mode`
- `/reset_service confirm`

## Storage

The bot uses `SQLite` and stores data in `sleepbot.db` by default.

Key tables:

- `families`
- `family_members`
- `children`
- `sleep_sessions`
- `reminder_settings`
- `custom_reminders`
- `invite_codes`
- `user_states`
- `notification_log`

## Deployment

This MVP is designed for a cheap and simple deployment:

- one Go binary
- one SQLite file
- no external queue
- long polling with Telegram API

For production you can run it under `systemd`, `pm2`, or any simple supervisor.

## Language

- [English](README.md)
- [Русский](README.ru.md)
