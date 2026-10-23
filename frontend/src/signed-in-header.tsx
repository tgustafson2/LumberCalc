import { SignOutButton } from "@clerk/clerk-react";
import { Link } from "@tanstack/react-router";

export function SignedInHeader(props: { readonly title: string }) {
  return (
    <header>
      <h1>{props.title}</h1>
      <nav>
        <Link to="/designs">Designs</Link>
        <Link to="/materials">Materials</Link>
        <Link to="/prices">Prices</Link>
      </nav>
      <SignOutButton />
    </header>
  );
}
