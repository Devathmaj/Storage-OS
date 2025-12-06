import { ThemeProvider } from "next-themes";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/AppSidebar";
import { Navbar } from "@/components/Navbar";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAuth } from "@/contexts/AuthContext";
import { OSNodeProvisioningSection } from "@/components/settings/OSNodeProvisioningSection";
import { EnrolledNodesSection } from "@/components/settings/EnrolledNodesSection";
import { PeerFileSharingSection } from "@/components/settings/PeerFileSharingSection";
import { StorageSection } from "@/components/settings/StorageSection";

const SettingsPage = () => {
  const breadcrumbs = ["Settings"];
  const { token, user } = useAuth();
  const isAdmin = user?.role === "admin";

  return (
    <ThemeProvider attribute="class" defaultTheme="light">
      <SidebarProvider>
        <div className="flex min-h-screen w-full">
          <AppSidebar />

          <div className="flex-1 flex flex-col">
            <div className="border-b bg-background/60 px-4 py-2">
              <SidebarTrigger />
            </div>

            <Navbar breadcrumbs={breadcrumbs} onSearch={() => {}} />

            <main className="p-6">
              <h1 className="text-2xl font-semibold mb-4">Settings</h1>
              
              <Tabs defaultValue="storage" className="space-y-6">
                <TabsList>
                  <TabsTrigger value="storage">Storage</TabsTrigger>
                  {isAdmin && <TabsTrigger value="storage-nodes">OS Storage Nodes</TabsTrigger>}
                  {isAdmin && <TabsTrigger value="os-nodes">OS Node Provisioning</TabsTrigger>}
                  {isAdmin && <TabsTrigger value="peer-sharing">Peer File Sharing</TabsTrigger>}
                </TabsList>

                <TabsContent value="storage" className="space-y-6">
                  <StorageSection />
                </TabsContent>

                {isAdmin && (
                  <>
                    <TabsContent value="storage-nodes" className="space-y-6">
                      <EnrolledNodesSection />
                    </TabsContent>

                    <TabsContent value="os-nodes" className="space-y-6">
                      <OSNodeProvisioningSection />
                    </TabsContent>

                    <TabsContent value="peer-sharing" className="space-y-6">
                      <PeerFileSharingSection />
                    </TabsContent>
                  </>
                )}
              </Tabs>
            </main>
          </div>
        </div>
      </SidebarProvider>
    </ThemeProvider>
  );
};

export default SettingsPage;
