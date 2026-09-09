import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ObserverDashboard, SyntheticObserverDataSource } from "../src";

const root = document.getElementById("root");

if (!root) {
  throw new Error("Dashboard root element is missing");
}

createRoot(root).render(
  <StrictMode>
    <ObserverDashboard dataSource={new SyntheticObserverDataSource()} mode="demo" />
  </StrictMode>
);
