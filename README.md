# Codelab Renderer

A lightweight, zero-dependency command-line tool written in Go that compiles a multi-file Markdown-based Codelab into a single, self-contained HTML file matching the official Google Developers Codelabs UI.

It works by parsing a main `instructions.lab.md` index file, resolving relative step links (e.g. `_01-introduction.md`), converting them from Markdown to HTML at compile time, and generating a single `preview.html` file using Google's official Codelab web components (`google-codelab`).

---

## Installation

### Option 1: Standalone Precompiled Binaries (No Go Required)
You can download precompiled standalone binaries for your operating system (such as macOS, Linux, or Windows) from the **Releases** tab.

1. Download the binary matching your system (e.g., `codelab-renderer-darwin-arm64` for Apple Silicon Mac).
2. Rename the downloaded file to `codelab-renderer`:
   ```bash
   mv codelab-renderer-darwin-arm64 codelab-renderer
   ```
3. Make the binary executable:
   ```bash
   chmod +x codelab-renderer
   ```
4. Move the binary into a directory in your system `PATH` (such as `~/bin` or `/usr/local/bin`):
   ```bash
   mv codelab-renderer ~/bin/
   ```

### Option 2: Compile from Source (Requires Go)
If you prefer compiling the utility from source or want to make local modifications:

#### Prerequisites
You must have [Go](https://go.dev/doc/install) installed (version 1.18 or higher is recommended).

#### Sub-Option A: Build to a Custom Directory (e.g. `~/bin`)
If you have a custom bin directory in your path (such as `~/bin` or `/usr/local/bin`), you can build the binary directly into it:

1. Clone or navigate to the repository directory.
2. Build the binary using the `-o` output flag:
   ```bash
   go build -o ~/bin/codelab-renderer
   ```
   *(Replace `~/bin/` with your preferred executable directory path, e.g., `/usr/local/bin/codelab-renderer` if you want it globally available for all users, which might require `sudo`).*

#### Sub-Option B: Use `go install`
Go can automatically build and install the binary to your Go binary directory (`$GOPATH/bin` or `$GOBIN`):

1. Run the following command from the repository root:
   ```bash
   go install .
   ```
2. Make sure your Go bin directory is added to your environment `PATH` in your shell configuration (e.g., `~/.zshrc` or `~/.bashrc`):
   ```bash
   export PATH=$PATH:$(go env GOPATH)/bin
   ```

---

## Usage

Navigate to the directory containing your Codelab files (the directory with `instructions.lab.md`) and run the tool:

```bash
codelab-renderer
```

### CLI Flags

You can customize the input and output file paths using flags:

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-in` | `instructions.lab.md` | Path to the main Codelab index markdown file |
| `-out` | `preview.html` | Path to the output compiled HTML file |
| `-open` | `true` | Automatically open the compiled Codelab HTML in the default web browser |
| `-watch` | `false` | Start a local HTTP server, watch all `.md` files in the directory, and automatically refresh the browser on saves |

### Preview Server with Hot Reload

If you want a live preview that refreshes automatically as you edit, run the tool in watch mode:

```bash
codelab-renderer -watch
```

This starts a local HTTP server, automatically maps local files/images, monitors the folder for changes to `.md` files, and reloads the browser tab instantly when you save changes in your editor.

#### Examples:
```bash
# Compile a specific markdown file and output to a custom HTML filename
codelab-renderer -in custom-instructions.md -out lab-preview.html

# Run watch server for a specific file
codelab-renderer -in custom-instructions.md -watch
```
