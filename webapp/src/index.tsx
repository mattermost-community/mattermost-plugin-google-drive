import {Store, Action} from 'redux';
import {GlobalState} from '@mattermost/types/lib/store';
import {Client4} from 'mattermost-redux/client';

import manifest from '@/manifest';
import {PluginRegistry} from '@/types/mattermost-webapp';

import {sendEphemeralPost} from './actions';

export default class Plugin {
    public async initialize(
        registry: PluginRegistry,
        store: Store<GlobalState, Action<Record<string, unknown>>>,
    ) {
        registry.registerPostDropdownMenuAction(
            'Upload file to Google Drive',
            async (postID: string) => {
                const fileInfos = await Client4.getFileInfosForPost(postID);
                if (fileInfos.length === 0) {
                    sendEphemeralPost(
                        'Selected post doesn\'t have any files to be uploaded',
                    )(store.dispatch, store.getState);
                    return;
                }
                const modal = {
                    url: `/plugins/${manifest.id}/api/v1/upload_file`,
                    dialog: {
                        callback_id: 'upload_file',
                        title: 'Upload to Google Drive',
                        elements: [
                            {
                                display_name: 'Select the files you\'d like to upload to Google Drive',
                                type: 'select',
                                name: 'fileID',
                                options: fileInfos.map((fileInfo) => ({
                                    text: fileInfo.name,
                                    value: fileInfo.id,
                                })),
                                optional: false,
                            },
                        ],
                        submit_label: 'Submit',
                        notify_on_cancel: true,
                        state: postID,
                    },
                };

                // Open the modal
                window.openInteractiveDialog(modal);
            },
        );

        registry.registerPostDropdownMenuAction(
            'Upload all files to Google Drive',
            async (postID: string) => {
                const fileInfos = await Client4.getFileInfosForPost(postID);
                if (fileInfos.length === 0) {
                    sendEphemeralPost(
                        'Selected post doesn\'t have any files to be uploaded',
                    )(store.dispatch, store.getState);
                    return;
                }
                const modal = {
                    url: `/plugins/${manifest.id}/api/v1/upload_all`,
                    dialog: {
                        callback_id: 'upload_all_files',
                        title: 'Upload all files Google Drive',
                        submit_label: 'Submit',
                        notify_on_cancel: true,
                        state: postID,
                    },
                };

                window.openInteractiveDialog(modal);
            },
        );
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
        openInteractiveDialog(modal: Record<string, unknown>): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
