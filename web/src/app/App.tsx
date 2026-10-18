import { RouterProvider } from 'react-router-dom';
import { router } from './router';

/** Root component: mounts the router (the shell is the router's root layout). */
export function App() {
  return <RouterProvider router={router} future={{ v7_startTransition: true }} />;
}
