# "AI-Box in 5 Minutes" -- Screen Recording Script

**Deliverable**: D8
**Format**: Narrated screen recording with captions
**Target Length**: 4:30-5:00
**Audience**: All developers (~200)
**Hosting**: Internal video platform or wiki

---

## Pre-Recording Checklist

- [ ] Clean Windows 11 desktop, no personal items visible
- [ ] VS Code installed with default theme (Dark+)
- [ ] AI-Box already set up (`aibox setup` completed)
- [ ] Sample project ready (e.g., a Spring Boot service or Node.js app)
- [ ] Screen resolution: 1920x1080
- [ ] Font size: 14pt in terminal, 14pt in VS Code (readable on small screens)
- [ ] Close all unnecessary applications and notifications
- [ ] Microphone tested, quiet environment

---

## Script and Storyboard

### Scene 1: Introduction (0:00 - 0:20)

**Visual**: Title card -- "AI-Box in 5 Minutes"

**Narration**:
> "AI-Box is your secure development sandbox. It lets you use AI coding tools like Claude Code and Codex while keeping source code contained. This video walks you through the entire workflow -- from starting a sandbox to pushing code."

**Visual**: Transition to desktop

---

### Scene 2: Starting the Sandbox (0:20 - 1:00)

**Visual**: Open Windows Terminal (or PowerShell)

**Narration**:
> "Starting AI-Box is one command. Open your terminal and type `aibox start`, passing your project workspace."

**Action**: Type and run:
```
aibox start --workspace ~/projects/my-service --toolpacks java@21
```

**Narration**:
> "This starts a secure container with your project and the Java 21 tool pack. The first time takes up to 90 seconds while images download. After that, warm starts take about 15 seconds."

**Visual**: Show the progress output. Wait for the "Sandbox ready" message.

**Narration**:
> "The sandbox is ready. Notice it tells you the SSH port and how to connect your IDE."

---

### Scene 3: Connecting VS Code (1:00 - 1:45)

**Visual**: Open VS Code

**Narration**:
> "VS Code detects the sandbox automatically via Remote SSH. Click the green remote indicator in the bottom-left corner, or use the command palette."

**Action**: Show Remote SSH connection. Select the `aibox` host. VS Code connects.

**Narration**:
> "VS Code Server runs inside the sandbox. Your extensions, terminal, and debugger all run inside the container -- your code never leaves."

**Visual**: Show the file explorer with project files in `/workspace`.

**Narration**:
> "Your project files are on a native Linux filesystem inside the sandbox. No file sync lag."

---

### Scene 4: Using AI Tools (1:45 - 2:45)

**Visual**: Open the integrated terminal in VS Code

**Narration**:
> "AI tools work out of the box. API keys are injected automatically -- you never see or manage them."

**Action**: Type `claude` in the terminal. Show Claude Code starting.

**Narration**:
> "Here I am using Claude Code. I will ask it to add a health check endpoint to this service."

**Action**: Type a prompt: "Add a /health endpoint that returns 200 OK with the application version"

**Visual**: Show Claude Code generating code, creating/editing files.

**Narration**:
> "Claude Code can read and edit files, run commands, and iterate -- all inside the sandbox. It cannot access anything outside /workspace, and all network traffic goes through the security proxy."

**Visual**: Show the generated code briefly.

---

### Scene 5: Building and Testing (2:45 - 3:30)

**Visual**: Terminal in VS Code

**Narration**:
> "Building works exactly like it does locally. Your build caches persist between sessions, so incremental builds are fast."

**Action**: Run `gradle build` (or `npm test` for a Node project). Show the build succeeding.

**Narration**:
> "Dependencies come from internal mirrors through the Nexus proxy. No direct internet access, no surprises."

**Action**: Run tests briefly. Show tests passing.

**Narration**:
> "Tests pass. The hot reload and debugging experience is identical to local development because everything runs natively inside the container."

---

### Scene 6: Pushing Code (3:30 - 4:00)

**Visual**: Terminal in VS Code

**Action**: Run:
```
git add .
git commit -m "feat: add health check endpoint"
git push
```

**Narration**:
> "Git push works as expected. Depending on your project policy, pushes may go through an approval flow. You can check the status with `aibox push status`."

**Visual**: Show the push completing (or show the approval pending message, then explain it).

---

### Scene 7: Self-Service Tools (4:00 - 4:30)

**Visual**: Terminal

**Narration**:
> "If something goes wrong, AI-Box has built-in diagnostics."

**Action**: Run `aibox doctor`. Show the output with all checks passing.

**Narration**:
> "`aibox doctor` checks your entire setup -- Podman, gVisor, proxy, DNS, image freshness, and more. If a check fails, it tells you how to fix it."

**Action**: Briefly show `aibox network test`.

**Narration**:
> "`aibox network test` verifies connectivity to all required services."

---

### Scene 8: Closing (4:30 - 5:00)

**Visual**: Return to desktop or title card

**Narration**:
> "That is AI-Box: start a sandbox, connect your IDE, use AI tools, build, test, and push -- all in a secure environment that feels like local development."
>
> "To get started, follow the quickstart guide for your IDE. If you hit any issues, run `aibox doctor` first, then check the troubleshooting FAQ. Your team's AI-Box champion is also available to help."
>
> "Links to all resources are in the video description."

**Visual**: End card with links:
- VS Code Quickstart
- JetBrains Quickstart
- Troubleshooting FAQ
- `#aibox-help` Slack channel

---

## Production Notes

### Recording Tools

- **Screen capture**: OBS Studio (free) or internal screen recording tool
- **Audio**: External microphone recommended; built-in laptop mic acceptable
- **Editing**: DaVinci Resolve (free) or internal video tool
- **Captions**: Auto-generated, then manually reviewed for accuracy

### Post-Production

1. Add title card and end card
2. Add captions (mandatory for accessibility)
3. Speed up wait times (image pull, build) to keep under 5 minutes
4. Add zoom-ins for terminal text that may be hard to read
5. Export at 1080p, H.264 codec

### Review Process

1. Draft recording reviewed by 2 champions for accuracy
2. Updated based on feedback
3. Final version approved by platform team lead
4. Published to internal video platform with searchable title and description

### Maintenance

Re-record when:
- The CLI interface changes significantly
- The IDE connection flow changes
- The video is more than 6 months old
