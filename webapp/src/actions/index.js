import {PostTypes} from 'mattermost-redux/action_types';
import {getCurrentChannelId, getCurrentUserId} from 'mattermost-redux/selectors/entities/common';

export function sendEphemeralPost(message) {
    return (dispatch, getState) => {
        const timestamp = Date.now();
        const state = getState();

        const post = {
            id: 'googleDrivePlugin' + Date.now(),
            user_id: getCurrentUserId(state),
            channel_id: getCurrentChannelId(state),
            message,
            type: 'system_ephemeral',
            create_at: timestamp,
            update_at: timestamp,
            root_id: '',
            parent_id: '',
            props: {},
        };

        dispatch({
            type: PostTypes.RECEIVED_NEW_POST,
            data: post,
        });
    };
}
