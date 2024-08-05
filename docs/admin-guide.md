## Create a Google Cloud Project

- Create a new Project. You would need to redirect to Google Cloud Console and select the option to New project. Then, select the name and the organization (optional).Select APIs. After creating a project, on the left side menu on APIs & Services, then, select the first option Enabled APIs & Services and wait, the page will redirect.
- Click on Enable APIs and Services option. Once you are in the API Library, search and enable following APIs:
    Google Drive API
    Google Docs API
    Google Slides API
    Google Sheets API
    Google Drive Activity API
- Go back to APIs & Services menu.Create a new OAuth consent screen. Select the option OAuth consent screen on the menu bar. If you would like to limit your application to organization-only users, select Internal, otherwise, select External option, then, fill the form with the data you would use for your project.
- Go back to APIs & Services menu.Create a new Client. Select the option Credentials, and on the menu bar, select Create credentials. A dropdown menu will be displayed, then, select OAuth Client ID option.
- Then, a select input will ask the type of Application type that will be used, select Web application, then, fill the form, and on Authorized redirect URIs add the following URI.
    Redirect URI: <mattermost_site_url>>/plugins/com.mattermost-community.plugin-google-drive/oauth/complete
- After the Client has been configured, on the main page of Credentials, on the submenu OAuth 2.0 Client IDs will be displayed the new Client and the info can be accessible whenever you need it.

# Admin guide

1. As a Mattermost system admin user, run the ``/google-drive setup`` command.
2. In the configuration modal, enter your Client ID and Client Secret.

