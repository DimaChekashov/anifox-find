import Link from "next/link";
import React from "react";

const links = [
    {href: "/", label: "Home"},
    {href: "/blog", label: "Blog"},
    {href: "/portfolio", label: "Portfolio"}
];

const Header: React.FC = () => {
    return (
        <header className="mb-10 flex justify-between p-4 border-b border-gray-200">
            <div>Fox Say Logo</div>
            <nav>
                <ul className="flex gap-4">
                    {links.map(({href, label}) => (
                        <li key={href}><Link href={href} className="hover:text-blue-500">{label}</Link></li>
                    ))}
                </ul>
            </nav>
        </header>
    );
}

export default Header;