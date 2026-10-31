export {};

declare global {
  interface Window {
    agentCollabDesktop?: {
      isDesktop: true;
      platform: "darwin" | "win32" | "linux" | string;
      getState: () => Promise<{
        configured: boolean;
        hubUrl?: string;
        hubUrls?: string[];
        hubId?: string;
        hubName?: string;
        runnerRegistered: boolean;
        runnerRunning: boolean;
        hubManaged: boolean;
        runnerId?: string;
        deviceId?: string;
        workspaceId?: string;
        platform: string;
        version: string;
      }>;
      configure: (input: { hubUrl: string }) => Promise<unknown>;
      openTeamLink?: (inviteLink: string) => Promise<boolean>;
      connectCurrentComputer: (input: { hubUrl: string; inviteCode: string }) => Promise<unknown>;
      reconnect: () => Promise<unknown>;
      openSettings: () => Promise<unknown>;
      listProjectBindings?: () => Promise<Array<{ stepId: string; workspaceId: string; directoryLabel: string; directoryPath: string; available: boolean; updatedAt: string }>>;
      bindProjectStep?: (input: { stepId: string; workspaceId: string }) => Promise<{ stepId: string; workspaceId: string; directoryLabel: string; directoryPath: string; available: boolean; updatedAt: string } | undefined>;
      unbindProjectStep?: (input: { stepId: string }) => Promise<boolean>;
      openProjectStep?: (input: { stepId: string }) => Promise<boolean>;
      onStatus: (listener: (state: unknown) => void) => () => void;
    };
  }
}
