# BELARUS CHAMP CLICKER

VIIPER-based virtual input clicker for Windows. Hold a trigger key to repeat virtual key presses and left mouse clicks through [VIIPER](https://github.com/Alia5/VIIPER) virtual HID devices.

## Layout

```
Experimental_Clicker/
  clicker.exe       ← built app (double-click to run)
  clicker/          ← source code
  VIIPER/           ← VIIPER dependency (git submodule)
```

Open the **`Experimental_Clicker`** folder in Cursor or VS Code — not the `VIIPER/` subfolder alone.

## Prerequisites

### USBIP on Windows (required)

VIIPER virtual devices need [usbip-win2](https://github.com/vadimgrn/usbip-win2). Install the kernel driver, then **reboot**.

Verify the driver loaded:

```powershell
Test-Path "$env:SystemRoot\System32\drivers\usbip2_ude.sys"
```

Or use the VIIPER install script (VIIPER + usbip-win2):

```powershell
irm https://alia5.github.io/VIIPER/stable/install.ps1 | iex
```

## Build

From the repo root:

```powershell
cd clicker
.\build.ps1
```

Output: `..\clicker.exe`

The build script compiles VIIPER into `VIIPER/dist/viiper.exe`, embeds it in the clicker, and produces the GUI binary.

After cloning, initialize the VIIPER submodule:

```powershell
git submodule update --init --recursive
```

## Run

Double-click `clicker.exe`, or:

```powershell
.\clicker.exe
```

The app embeds the VIIPER server and starts it when you click **Start**. No separate server terminal is needed.

### GUI

- **Trigger keys** — click "Add key...", then press a supported key (no keys bound by default)
- **Clear keys** — remove all bound keys
- **Delay (ms)** — pause between loop steps (button hold uses at least 60 ms)
- **Start / Stop** — run or stop the clicker
- **ESC** — emergency stop while running
- **Logs** — live output in the window

### Default loop

While the trigger key is held (`runner/runner.go`):

1. Send virtual trigger key (down + up)
2. Sleep (delay ms)
3. Send virtual left click (down + up)
4. Sleep (delay ms)
5. Repeat until the key is released

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `usbip-win2 driver not found` | Install usbip-win2, reboot |
| `409 Conflict: Failed to auto-attach device` | Fix USBIP setup, restart clicker |
| `VIIPER connection failed` | Click Start again; check logs for server errors |
| Loop never triggers | Ensure the physical trigger key works (`GetAsyncKeyState`) |
| Game ignores virtual mouse | Use 60+ ms delay; start clicker before launching the game |

## Development

| Path | Purpose |
|------|---------|
| `clicker/runner/` | VIIPER client, input loop, key mappings |
| `clicker/gui/` | Walk GUI, embedded VIIPER server lifecycle |
| `VIIPER/` | Upstream VIIPER (local `replace` in `clicker/go.mod`) |
