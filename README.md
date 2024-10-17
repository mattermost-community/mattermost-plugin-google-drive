# Mattermost Google Drive Plugin

[![Release](https://img.shields.io/github/v/release/mattermost/mattermost-plugin-google-drive)](https://github.com/mattermost-community/mattermost-plugin-google-drive/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/mattermost-community/mattermost-plugin-google-drive)](https://goreportcard.com/report/github.com/mattermost-community/mattermost-plugin-google-drive)
[![HW](https://img.shields.io/github/issues/mattermost/mattermost-plugin-google-drive/Up%20For%20Grabs?color=dark%20green&label=Help%20Wanted)](https://github.com/mattermost-community/mattermost-plugin-google-drive/issues?q=is%3Aissue+is%3Aopen+sort%3Aupdated-desc+label%3A%22Up+For+Grabs%22+label%3A%22Help+Wanted%22)
[![Mattermost Community Channel](https://img.shields.io/badge/Mattermost%20Community-~Plugin%3A%20GoogleDrive-blue)](https://community.mattermost.com/core/channels/plugin-googledrive)

**Help Wanted Tickets [here](https://github.com/mattermost-community/mattermost-plugin-google-drive/issues)**

# Contents

- [Overview](#overview)
- [Features](#features)
- [Admin Guide](docs/admin-guide.md)
- [End User Guide](#end-user-guide)
- [Contribute](#contribute)
- [Security Vulnerability Disclosure](#security-vulnerability-disclosure)
- [Get Help](#get-help)

## Overview
This plugin allows you to integrate Google Drive to your Mattermost instance, letting you:
- Create a Google Drive file
- Share a Google Drive file
- View and reply to comments
- Upload any file attached to a Mattermost post directly to your Drive
- Enable or disable notifications for comments and files permission changes

## [Admin Guide](docs/admin-guide.md)

## End User Guide

### Get Started

### Use the Plugin

### Slash commands

After your System Admin has configured the Google Drive plugin, run `/google-drive connect` in a Mattermost channel to connect your Mattermost and Google accounts.

#### Connect to your Google account

|                        |                                                 |
| -----------------------| ------------------------------------------------|
| `/google-drive connect`       | Connect your Mattermost account to Google.      |
| `/google-drive disconnect`    | Disconnect your Mattermost account from Google. |

#### Create Google Drive files

|                           |                                             |
| ------------------------- | ------------------------------------------- |
| `/google-drive create doc`    | Create and share a new Google document.     |
| `/google-drive create sheet`  | Create and share a new Google spreadsheet.  |
| `/google-drive create slide`  | Create and share a new Google presentation. |

#### Subscribe yourself to notifications

|                                         |                                               |
| ----------------------------------------| --------------------------------------------- |
| `/google-drive notifications start`            | Enable Google Drive activity notifications.   |
| `/google-drive notifications stop`             | Disable Google Drive activity notifications.  |

#### Post menu bindings
`Upload file to Google drive`:: This option is available in any post to upload any attached files directly to your Google Drive. You will be prompted with a dropdown to choose the attachment you want to upload.
`Upload all files to Google drive`:: This option is available in any post to upload all the attached files directly to your Google Drive. You will be prompted with a confirmation to upload all the attached files.


#### How do I share feedback on this plugin?

Wanting to share feedback on this plugin?

Feel free to create a [GitHub Issue](https://github.com/mattermost-community/mattermost-plugin-google-drive/issues) or join the [Google Drive Plugin channel](https://community.mattermost.com/core/channels/plugin-googledrive) on the Mattermost Community server to discuss.

## Contribute

### I saw a bug, I have a feature request or a suggestion

Please fill a [GitHub issue](https://github.com/mattermost-community/mattermost-plugin-google-drive/issues/new/choose), it will be very useful!

### Development

Pull Requests are welcome! You can contact us on the [Mattermost Community ~Plugin: GoogleDrive](https://community.mattermost.com/core/channels/plugin-googledrive).

This plugin contains server and webapp both portions. Read our documentation about the [Developer Workflow](https://developers.mattermost.com/extend/plugins/developer-workflow/) and [Developer Setup](https://developers.mattermost.com/extend/plugins/developer-setup/) for more information about developing and extending plugins.

To avoid having to manually install your plugin, build and deploy your plugin using one of the following options.

### Deploy with local mode

If your Mattermost server is running locally, you can enable [local mode](https://docs.mattermost.com/administration/mmctl-cli-tool.html#local-mode) to streamline deploying your plugin. After configuring it, just run:

## Security vulnerability disclosure

Please report any security vulnerability to [https://mattermost.com/security-vulnerability-report/](https://mattermost.com/security-vulnerability-report/).

## Get Help

For questions, suggestions, and help, visit the  [Google Drive Plugin channel](https://community.mattermost.com/core/channels/plugin-googledrive) on our Community server.
