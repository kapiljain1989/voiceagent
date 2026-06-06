import type { Metadata } from "next";
import { JetBrains_Mono, DM_Sans } from "next/font/google";
import "./globals.css";
import { Sidebar } from "@/components/layout/Sidebar";

const dmSans = DM_Sans({
  variable: "--font-sans",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

const jetbrains = JetBrains_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

export const metadata: Metadata = {
  title: "VoiceAgent Command",
  description: "AI-Powered Call Center Operations Platform",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className={`${dmSans.variable} ${jetbrains.variable} h-full antialiased`}>
      <body className="min-h-full flex bg-[#0a0e1a]">
        <Sidebar />
        <main className="flex-1 ml-16 lg:ml-56 min-h-screen">
          <div className="p-6 lg:p-8">{children}</div>
        </main>
      </body>
    </html>
  );
}
