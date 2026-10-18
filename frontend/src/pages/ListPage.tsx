import { ShoppingCart } from "lucide-react";
import { Link } from "react-router";
import { useShell } from "../components/AppShell";
import { Card } from "../components/ui";

// Placeholder until stage 2 (shopping list) lands.
export function ListPage() {
  const { household } = useShell();
  return (
    <div className="pt-2">
      <Card className="py-10 text-center">
        <span className="mx-auto mb-4 flex size-14 items-center justify-center rounded-2xl bg-accent/15 text-accent">
          <ShoppingCart className="size-7" />
        </span>
        <h2 className="text-xl font-bold">Список покупок — на следующем этапе</h2>
        <p className="mx-auto mt-2 max-w-xs text-muted">
          А пока позовите родных в «{household.name}», чтобы список сразу был общим.
        </p>
        <Link to="/family" className="mt-5 inline-block font-semibold text-accent">
          Пригласить семью →
        </Link>
      </Card>
    </div>
  );
}
