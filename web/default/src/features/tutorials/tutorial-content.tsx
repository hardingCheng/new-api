/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { TFunction } from 'i18next'
import {
  Braces,
  Image,
  type LucideIcon,
  MonitorCog,
  Settings,
  Sparkles,
  TerminalSquare,
} from 'lucide-react'

export type TutorialCodeBlock = {
  label: string
  code: string
}

export type TutorialStep = {
  title: string
  description: string
  codeBlocks?: TutorialCodeBlock[]
  tabGroups?: TutorialTabGroup[]
  note?: string
}

export type TutorialTab = {
  value: string
  label: string
  title?: string
  description?: string
  codeBlocks?: TutorialCodeBlock[]
  tabGroups?: TutorialTabGroup[]
  actionLabel?: string
  actionUrl?: string
  note?: string
}

export type TutorialTabGroup = {
  label?: string
  ariaLabel: string
  defaultValue: string
  tabs: TutorialTab[]
}

export type Tutorial = {
  slug: string
  title: string
  shortTitle: string
  description: string
  category: string
  duration: string
  level: string
  icon: LucideIcon
  accentClassName: string
  officialUrl?: string
  steps: TutorialStep[]
}

export function getTutorials(t: TFunction, origin: string): Tutorial[] {
  const apiBaseUrl = `${origin}/v1`

  return [
    {
      slug: 'quick-start',
      title: t('API Quick Start'),
      shortTitle: t('API Quick Start'),
      description: t(
        'Create a key, copy the API address, and complete your first request in minutes.'
      ),
      category: t('Getting Started'),
      duration: t('3 minutes'),
      level: t('Beginner'),
      icon: Braces,
      accentClassName: 'bg-identity-blue/15 text-identity-blue',
      steps: [
        {
          title: t('Create an API key'),
          description: t(
            'Open API Keys from the sidebar, create a key, and copy it immediately. For security, the complete key may only be shown once.'
          ),
          note: t(
            'Never send your complete API key to customer support or paste it into public screenshots.'
          ),
        },
        {
          title: t('Confirm the API address'),
          description: t(
            'The address below automatically follows the station you are currently using.'
          ),
          codeBlocks: [{ label: 'Base URL', code: apiBaseUrl }],
        },
        {
          title: t('Send your first request'),
          description: t(
            'Replace the placeholder key and model with a model available to your account, then run the request in a terminal.'
          ),
          codeBlocks: [
            {
              label: 'cURL',
              code: `curl ${apiBaseUrl}/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer sk-your-api-key" \\
  -d '{
    "model": "gpt-5.6",
    "messages": [{"role": "user", "content": "Say hello in one sentence."}]
  }'`,
            },
          ],
          note: t(
            'If a model is unavailable, open Model Pricing and copy another model name that is enabled for your account.'
          ),
        },
      ],
    },
    {
      slug: 'codex',
      title: t('Codex Setup'),
      shortTitle: 'Codex',
      description: t(
        'Install Codex CLI and connect it to this station through a custom Responses API provider.'
      ),
      category: t('Coding Tools'),
      duration: t('10 minutes'),
      level: t('Beginner'),
      icon: TerminalSquare,
      accentClassName: 'bg-identity-green/15 text-identity-green',
      officialUrl: 'https://developers.openai.com/codex/cli',
      steps: [
        {
          title: t('Choose a Codex client and install it'),
          description: t(
            'Codex CLI, the VS Code extension, and the desktop app can share the same user-level configuration. Choose the surface you plan to use first.'
          ),
          tabGroups: [
            {
              label: t('Choose a client'),
              ariaLabel: t('Codex client'),
              defaultValue: 'cli',
              tabs: [
                {
                  value: 'cli',
                  label: 'Codex CLI',
                  title: t('Use Codex from a terminal'),
                  description: t(
                    'Best for terminal-first development, local automation, and repeatable commands.'
                  ),
                  tabGroups: [
                    {
                      label: t('Choose your operating system'),
                      ariaLabel: t('Operating system'),
                      defaultValue: 'macos-linux',
                      tabs: [
                        {
                          value: 'macos-linux',
                          label: 'macOS / Linux',
                          title: t('Official standalone installer'),
                          description: t(
                            'Open Terminal and run the official installer. Run the same command later to update Codex.'
                          ),
                          codeBlocks: [
                            {
                              label: 'Terminal',
                              code: 'curl -fsSL https://chatgpt.com/codex/install.sh | sh',
                            },
                          ],
                        },
                        {
                          value: 'windows',
                          label: 'Windows',
                          title: t('Install with npm in PowerShell'),
                          description: t(
                            'Install Node.js first, then open PowerShell as your normal user and install Codex globally.'
                          ),
                          codeBlocks: [
                            {
                              label: 'PowerShell',
                              code: 'npm install -g @openai/codex@latest',
                            },
                          ],
                          actionLabel: t('Download Node.js'),
                          actionUrl: 'https://nodejs.org/en/download',
                          note: t(
                            'If native Windows tooling causes compatibility problems, run Codex inside WSL and follow the Linux instructions.'
                          ),
                        },
                        {
                          value: 'npm',
                          label: 'npm',
                          title: t('Cross-platform npm installation'),
                          description: t(
                            'Use this method when Node.js is already installed on Windows, macOS, or Linux.'
                          ),
                          codeBlocks: [
                            {
                              label: 'npm',
                              code: 'npm install -g @openai/codex@latest',
                            },
                          ],
                        },
                      ],
                    },
                  ],
                },
                {
                  value: 'vscode',
                  label: t('VS Code extension'),
                  title: t('Use Codex beside your code'),
                  description: t(
                    'Install the official OpenAI extension in VS Code, Cursor, or Windsurf. The command below installs it in VS Code.'
                  ),
                  codeBlocks: [
                    {
                      label: 'VS Code CLI',
                      code: 'code --install-extension openai.chatgpt',
                    },
                  ],
                  actionLabel: t('Open VS Code Marketplace'),
                  actionUrl:
                    'https://marketplace.visualstudio.com/items?itemName=openai.chatgpt',
                  note: t(
                    'After installation, open the Command Palette and run “Codex: Open Codex Sidebar” if the Codex icon is not visible.'
                  ),
                },
                {
                  value: 'desktop',
                  label: t('Desktop app'),
                  title: t('Use Codex in the desktop app'),
                  description: t(
                    'Download the current ChatGPT desktop app, which includes Codex workflows for local projects and long-running work.'
                  ),
                  actionLabel: t('Open official download page'),
                  actionUrl:
                    'https://developers.openai.com/codex/quickstart?setup=app',
                  note: t(
                    'Install the app under the same operating-system user account so it can use the same Codex configuration and credential store.'
                  ),
                },
              ],
            },
          ],
        },
        {
          title: t('Open the Codex configuration folder'),
          description: t(
            'Create the user-level Codex folder and open it in your file manager. The path is different on Windows, macOS, and Linux.'
          ),
          tabGroups: [
            {
              ariaLabel: t('Operating system'),
              defaultValue: 'windows',
              tabs: [
                {
                  value: 'windows',
                  label: 'Windows',
                  title: t('Open the folder in File Explorer'),
                  description: t(
                    'Run both commands in PowerShell. The first creates the folder if it does not exist.'
                  ),
                  codeBlocks: [
                    {
                      label: 'PowerShell',
                      code: `New-Item -ItemType Directory -Force "$env:USERPROFILE\\.codex"
Start-Process "$env:USERPROFILE\\.codex"`,
                    },
                    {
                      label: t('Configuration file path'),
                      code: '%USERPROFILE%\\.codex\\config.toml',
                    },
                  ],
                },
                {
                  value: 'macos',
                  label: 'macOS',
                  title: t('Open the folder in Finder'),
                  description: t(
                    'Run the command in Terminal to create the folder and open it in Finder.'
                  ),
                  codeBlocks: [
                    {
                      label: 'Terminal',
                      code: 'mkdir -p ~/.codex && open ~/.codex',
                    },
                    {
                      label: t('Configuration file path'),
                      code: '~/.codex/config.toml',
                    },
                  ],
                },
                {
                  value: 'linux',
                  label: 'Linux',
                  title: t('Open the configuration directory'),
                  description: t(
                    'Create the directory first. If your desktop supports xdg-open, the second command opens it in the file manager.'
                  ),
                  codeBlocks: [
                    {
                      label: 'Terminal',
                      code: 'mkdir -p ~/.codex && xdg-open ~/.codex',
                    },
                    {
                      label: t('Configuration file path'),
                      code: '~/.codex/config.toml',
                    },
                  ],
                },
              ],
            },
          ],
          note: t(
            'The provider configuration belongs in your user-level config.toml, not a project-level .codex/config.toml.'
          ),
        },
        {
          title: t('Create config.toml and save your API key'),
          description: t(
            'Configure this station as a custom Responses API provider, then let Codex securely create its credential cache.'
          ),
          tabGroups: [
            {
              label: t('Configure each file'),
              ariaLabel: t('Configuration file'),
              defaultValue: 'config',
              tabs: [
                {
                  value: 'config',
                  label: 'config.toml',
                  title: t('Paste this provider configuration'),
                  description: t(
                    'Create config.toml in the folder from the previous step. The API address automatically matches the station you are visiting.'
                  ),
                  codeBlocks: [
                    {
                      label: 'config.toml',
                      code: `model_provider = "station"
model = "gpt-5.6-sol"
model_reasoning_effort = "high"

[model_providers.station]
name = "Current Station"
base_url = "${apiBaseUrl}"
requires_openai_auth = true
wire_api = "responses"`,
                    },
                  ],
                  note: t(
                    'Choose another enabled model from Model Pricing if gpt-5.6-sol is not available to your account.'
                  ),
                },
                {
                  value: 'login',
                  label: t('Save API key'),
                  title: t('Let Codex store the key for all local clients'),
                  description: t(
                    'Run the command for your operating system and replace the placeholder with an API key created on this station.'
                  ),
                  tabGroups: [
                    {
                      ariaLabel: t('Operating system'),
                      defaultValue: 'macos-linux',
                      tabs: [
                        {
                          value: 'macos-linux',
                          label: 'macOS / Linux',
                          codeBlocks: [
                            {
                              label: 'Terminal',
                              code: `printf '%s' 'sk-your-api-key' | codex login --with-api-key`,
                            },
                          ],
                        },
                        {
                          value: 'windows',
                          label: 'Windows',
                          codeBlocks: [
                            {
                              label: 'PowerShell',
                              code: '"sk-your-api-key" | codex login --with-api-key',
                            },
                          ],
                        },
                      ],
                    },
                  ],
                  note: t(
                    'Do not enter your station account password. Codex only needs the API key that starts with sk-.'
                  ),
                },
                {
                  value: 'auth',
                  label: 'auth.json',
                  title: t('Do not manually create or share auth.json'),
                  description: t(
                    'Codex creates its login cache automatically. Depending on your settings, credentials are stored in the operating-system keyring or in auth.json.'
                  ),
                  codeBlocks: [
                    {
                      label: 'macOS / Linux',
                      code: '~/.codex/auth.json',
                    },
                    {
                      label: 'Windows',
                      code: '%USERPROFILE%\\.codex\\auth.json',
                    },
                  ],
                  note: t(
                    'Treat auth.json like a password: never upload it, commit it to Git, paste it into support chats, or include it in screenshots.'
                  ),
                },
              ],
            },
          ],
        },
        {
          title: t('Restart your client and verify the connection'),
          description: t(
            'Fully restart the client after changing the provider, then confirm that Codex loaded this station and the selected model.'
          ),
          tabGroups: [
            {
              ariaLabel: t('Codex client'),
              defaultValue: 'cli',
              tabs: [
                {
                  value: 'cli',
                  label: 'Codex CLI',
                  title: t('Start Codex inside a project'),
                  codeBlocks: [
                    {
                      label: 'Terminal / PowerShell',
                      code: `cd PATH_TO_YOUR_PROJECT
codex`,
                    },
                    { label: t('Inside Codex'), code: '/status' },
                  ],
                  note: t(
                    'The status output should show the station provider and your selected model.'
                  ),
                },
                {
                  value: 'vscode',
                  label: t('VS Code extension'),
                  title: t('Reload the editor window'),
                  description: t(
                    'Open the project, run “Developer: Reload Window”, then open the Codex sidebar. The extension uses the same local Codex configuration.'
                  ),
                  codeBlocks: [
                    { label: 'Terminal', code: 'code PATH_TO_YOUR_PROJECT' },
                  ],
                },
                {
                  value: 'desktop',
                  label: t('Desktop app'),
                  title: t('Quit and reopen the desktop app'),
                  description: t(
                    'Quit the app completely, reopen it, select a local project folder, and start a new Codex task.'
                  ),
                },
              ],
            },
          ],
          note: t(
            'Restart Codex after changing providers so the model catalog and connection settings are reloaded.'
          ),
        },
        {
          title: t('Troubleshoot common connection problems'),
          description: t(
            'Check the client version, login state, station API, and model availability before changing the configuration again.'
          ),
          codeBlocks: [
            {
              label: t('Client checks'),
              code: `codex --version
codex login status`,
            },
            {
              label: t('Station API check'),
              code: `curl ${apiBaseUrl}/models \\
  -H "Authorization: Bearer sk-your-api-key"`,
            },
          ],
          note: t(
            '401 usually means the API key is invalid; 404 often means the base URL is wrong; a model error means you should choose another model from Model Pricing. If settings look unchanged, fully quit every Codex client and start it again.'
          ),
        },
      ],
    },
    {
      slug: 'claude-code',
      title: t('Claude Code Setup'),
      shortTitle: 'Claude Code',
      description: t(
        'Install Claude Code and point its Anthropic-compatible requests to this station.'
      ),
      category: t('Coding Tools'),
      duration: t('5 minutes'),
      level: t('Beginner'),
      icon: Sparkles,
      accentClassName: 'bg-identity-orange/15 text-identity-orange',
      officialUrl: 'https://code.claude.com/docs/en/getting-started',
      steps: [
        {
          title: t('Install Claude Code'),
          description: t(
            'Use the official native installer recommended for your operating system.'
          ),
          codeBlocks: [
            {
              label: 'macOS / Linux / WSL',
              code: 'curl -fsSL https://claude.ai/install.sh | bash',
            },
            {
              label: 'Windows PowerShell',
              code: 'irm https://claude.ai/install.ps1 | iex',
            },
          ],
        },
        {
          title: t('Configure the API address and key'),
          description: t(
            'Set the station address and your API key in the terminal before starting Claude Code.'
          ),
          codeBlocks: [
            {
              label: 'macOS / Linux / WSL',
              code: `export ANTHROPIC_BASE_URL="${origin}"
export ANTHROPIC_AUTH_TOKEN="sk-your-api-key"`,
            },
            {
              label: 'Windows PowerShell',
              code: `$env:ANTHROPIC_BASE_URL="${origin}"
$env:ANTHROPIC_AUTH_TOKEN="sk-your-api-key"`,
            },
          ],
          note: t(
            'Use an API key created on this station. Do not use your station account password here.'
          ),
        },
        {
          title: t('Start and verify'),
          description: t(
            'Run Claude Code inside a project directory, then ask a simple question to verify the connection.'
          ),
          codeBlocks: [
            { label: t('Start'), code: 'claude' },
            {
              label: t('Connection check'),
              code: 'Explain the purpose of this repository in three bullet points.',
            },
          ],
        },
      ],
    },
    {
      slug: 'gemini-cli',
      title: t('Gemini CLI Setup'),
      shortTitle: 'Gemini CLI',
      description: t(
        'Install Gemini CLI and use its supported base URL override with your station key.'
      ),
      category: t('Coding Tools'),
      duration: t('5 minutes'),
      level: t('Beginner'),
      icon: MonitorCog,
      accentClassName: 'bg-identity-purple/15 text-identity-purple',
      officialUrl: 'https://github.com/google-gemini/gemini-cli',
      steps: [
        {
          title: t('Install Gemini CLI'),
          description: t(
            'Install globally with npm, or use npx if you only want to try it once.'
          ),
          codeBlocks: [
            { label: 'npm', code: 'npm install -g @google/gemini-cli@latest' },
            { label: 'npx', code: 'npx @google/gemini-cli' },
          ],
        },
        {
          title: t('Configure the Gemini endpoint'),
          description: t(
            'Gemini CLI supports an HTTPS base URL override when API-key authentication is used.'
          ),
          codeBlocks: [
            {
              label: 'macOS / Linux',
              code: `export GOOGLE_GEMINI_BASE_URL="${origin}"
export GEMINI_API_KEY="sk-your-api-key"`,
            },
            {
              label: 'Windows PowerShell',
              code: `$env:GOOGLE_GEMINI_BASE_URL="${origin}"
$env:GEMINI_API_KEY="sk-your-api-key"`,
            },
          ],
        },
        {
          title: t('Start Gemini CLI'),
          description: t(
            'Start the client in your project. If prompted for authentication, choose API key.'
          ),
          codeBlocks: [{ label: t('Start'), code: 'gemini' }],
          note: t(
            'Choose a Gemini model that is visible in Model Pricing. Availability can differ by station and account group.'
          ),
        },
      ],
    },
    {
      slug: 'cc-switch',
      title: t('CC Switch Setup'),
      shortTitle: 'CC Switch',
      description: t(
        'Manage Codex, Claude Code, and Gemini CLI providers from one visual desktop app.'
      ),
      category: t('Configuration Tools'),
      duration: t('4 minutes'),
      level: t('Beginner'),
      icon: Settings,
      accentClassName: 'bg-identity-cyan/15 text-identity-cyan',
      officialUrl: 'https://github.com/farion1231/cc-switch/releases',
      steps: [
        {
          title: t('Download CC Switch'),
          description: t(
            'Only download CC Switch from its official GitHub Releases page. The app is free and open source.'
          ),
          codeBlocks: [
            { label: 'macOS Homebrew', code: 'brew install --cask cc-switch' },
            {
              label: 'Windows / Linux',
              code: 'https://github.com/farion1231/cc-switch/releases',
            },
          ],
          note: t(
            'Ignore websites or apps that charge for CC Switch or ask for your station login password.'
          ),
        },
        {
          title: t('Add this station as a provider'),
          description: t(
            'Choose the target app, add a custom provider, and fill in the values below.'
          ),
          codeBlocks: [
            {
              label: 'Codex / OpenAI-compatible',
              code: `${t('Provider name')}: ${t('Current Station')}
${t('API address')}: ${apiBaseUrl}
${t('API key')}: sk-your-api-key
${t('Model')}: ${t('Choose an available model from Model Pricing')}`,
            },
            {
              label: 'Claude Code',
              code: `${t('Provider name')}: ${t('Current Station')}
${t('API address')}: ${origin}
${t('API key')}: sk-your-api-key
${t('Model')}: ${t('Choose an available model from Model Pricing')}`,
            },
            {
              label: 'Gemini CLI',
              code: `${t('Provider name')}: ${t('Current Station')}
${t('API address')}: ${origin}
${t('API key')}: sk-your-api-key
${t('Model')}: ${t('Choose an available model from Model Pricing')}`,
            },
          ],
        },
        {
          title: t('Activate and restart the client'),
          description: t(
            'Click Activate for the provider. Restart Codex or Gemini CLI after switching so they reload their configuration.'
          ),
          note: t(
            'CC Switch keeps backups of the configuration files it manages, but you should still avoid editing the same file in two tools at once.'
          ),
        },
      ],
    },
    {
      slug: 'image-generation',
      title: t('Image Generation'),
      shortTitle: t('Image Generation'),
      description: t(
        'Choose an enabled image model and generate your first image through the compatible API.'
      ),
      category: t('API Tutorials'),
      duration: t('4 minutes'),
      level: t('Beginner'),
      icon: Image,
      accentClassName: 'bg-identity-pink/15 text-identity-pink',
      steps: [
        {
          title: t('Choose an image model'),
          description: t(
            'Open Model Pricing, filter for image models, and copy a model name that is available to your account.'
          ),
        },
        {
          title: t('Send an image generation request'),
          description: t(
            'Replace the model and API key placeholders, then describe the image you want in the prompt.'
          ),
          codeBlocks: [
            {
              label: 'cURL',
              code: `curl ${apiBaseUrl}/images/generations \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer sk-your-api-key" \\
  -d '{
    "model": "YOUR_IMAGE_MODEL",
    "prompt": "A calm lakeside cabin at sunrise, editorial photography",
    "size": "1024x1024"
  }'`,
            },
          ],
        },
        {
          title: t('Save the result'),
          description: t(
            'The response may contain an image URL or base64 image data, depending on the selected model. Save the result before a temporary URL expires.'
          ),
          note: t(
            'Image sizes and supported parameters vary by model. If a parameter is rejected, remove optional fields and retry with only model and prompt.'
          ),
        },
      ],
    },
  ]
}
