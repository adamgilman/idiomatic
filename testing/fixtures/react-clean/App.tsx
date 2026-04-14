import React, { useState, useEffect } from "react";

interface Item {
  id: string;
  name: string;
}

interface AppProps {
  initialItems?: Item[];
}

function ItemCard({ item }: { item: Item }) {
  return <div className="item-card">{item.name}</div>;
}

export function App({ initialItems = [] }: AppProps): React.JSX.Element {
  const [items, setItems] = useState<Item[]>(initialItems);
  const [filter, setFilter] = useState("");

  useEffect(() => {
    document.title = `Items: ${items.length}`;
  }, [items.length]);

  const filteredItems = items.filter((item) =>
    item.name.toLowerCase().includes(filter.toLowerCase())
  );

  function handleAddItem() {
    const newItem: Item = { id: crypto.randomUUID(), name: "New Item" };
    setItems((prev) => [...prev, newItem]);
  }

  return (
    <div>
      <input
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        placeholder="Filter items"
      />
      <button onClick={handleAddItem}>Add Item</button>
      <ul>
        {filteredItems.map((item) => (
          <li key={item.id}>
            <ItemCard item={item} />
          </li>
        ))}
      </ul>
    </div>
  );
}

export default App;
