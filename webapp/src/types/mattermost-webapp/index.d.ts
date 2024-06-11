export interface PluginRegistry {
    registerPostTypeComponent(typeName: string, component: React.ElementType)
    registerPostDropdownMenuAction(text: React.ReactNode | React.ElementType, action?: (...args: any) => void, filter?: (id: string) => boolean)

    // Add more if needed from https://developers.mattermost.com/extend/plugins/webapp/reference
}
