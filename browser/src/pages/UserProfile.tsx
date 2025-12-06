import { ThemeProvider } from "next-themes";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/AppSidebar";
import { Navbar } from "@/components/Navbar";

const UserProfile = () => {
  const breadcrumbs = ["Profile"];

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
              <h1 className="text-2xl font-semibold mb-4">User Profile</h1>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="p-6 bg-card rounded-md">
                  <h2 className="font-medium">Account</h2>
                  <p className="text-sm text-muted-foreground mt-2">Name: Jane Doe</p>
                  <p className="text-sm text-muted-foreground">Email: jane@example.com</p>
                </div>

                <div className="p-6 bg-card rounded-md">
                  <h2 className="font-medium">Preferences</h2>
                  <p className="text-sm text-muted-foreground mt-2">Theme, notifications, and other preferences go here.</p>
                </div>
              </div>
            </main>
          </div>
        </div>
      </SidebarProvider>
    </ThemeProvider>
  );
};

export default UserProfile;
